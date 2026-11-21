package databases

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	mariadb "github.com/aantich/go-mariadb"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/data"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/sealer"
)

// Provisioner is the worker step that provisions or verifies pending database
// resources, one per run, rechecking a requesting agent's live authority first.
// Tenants are served round-robin within one leadership term.
type Provisioner struct {
	DB     *sql.DB
	Access *access.Checker
	Sealer *sealer.Sealer
	// Connect opens a database on a target with the given credentials.
	Connect func(ctx context.Context, target DatabaseEndpoint, cred Credentials, name string) (*sql.DB, error)
	// Warn logs a failure without its message (see httpapi.Diagnostic).
	Warn func(ctx context.Context, msg string, err error, attrs ...any)
	last string // Tenant of the previous job in this leadership term
}

func (p *Provisioner) Name() string { return "provisioning" }

// Start begins a leadership term: an attempt interrupted by a previous leader
// returns to pending, and the Tenant round-robin restarts.
func (p *Provisioner) Start(ctx context.Context, conn *sql.Conn) error {
	p.last = ""
	_, e := conn.ExecContext(ctx, "UPDATE database_resources SET state='pending',revision=revision+1 WHERE state='provisioning'")
	return e
}

// Run provisions the next pending resource using the worker's pool db. It
// reports busy when it handled a job, so the leader polls again without idling.
func (p *Provisioner) Run(ctx context.Context, db *sql.DB) (bool, error) {
	job, e := NextJob(ctx, db, p.last)
	if e != nil {
		var ae *registry.APIError
		if !errors.As(e, &ae) || ae.Status != 404 {
			return false, e
		}
		return false, nil
	}
	record, tenant := job.ID, job.TenantID
	jobCtx := op.AgentAuditContext(ctx, job.AgentGrant, job.AgentClient)
	p.last = tenant
	tx, e := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return true, e
	}
	var active string
	e = tx.QueryRowContext(ctx, "SELECT state FROM tenants WHERE id=? FOR UPDATE", tenant).Scan(&active)
	if e == nil {
		var eligible bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM database_resources
			 WHERE id=? AND tenant_id=? AND state='pending' AND preview_cleanup_at IS NULL)`, record, tenant).Scan(&eligible)
		if e == nil && !eligible {
			tx.Rollback()
			return true, nil // The selected resource was archived or changed.
		}
	}
	if e == nil && job.AgentGrant != "" {
		res, permission := access.Resource{Tenant: tenant}, "databases.provision"
		var production string
		if registry.Lookup(ctx, tx, "SELECT w.production_space_id FROM development_workspaces w JOIN spaces sp ON sp.id=w.backing_space_id WHERE sp.database_resource_id=? AND w.state='preparing'", []any{record}, &production) == nil {
			res.Space = production
			permission = "workspaces.write"
		}
		if denied := p.Access.AgentJobAllowed(ctx, tx, job.AgentGrant, job.AgentUser, res, permission, job.TargetID); denied != nil {
			var ae *registry.APIError
			if !errors.As(denied, &ae) {
				tx.Rollback()
				return true, denied
			}
			_, e = tx.ExecContext(ctx, "UPDATE database_resources SET state='failed',last_error_code='agent_access_revoked',last_error_message='Agent authority expired or was revoked before this job started.',revision=revision+1 WHERE id=? AND state='pending'", record)
			if e != nil {
				tx.Rollback()
				return true, e
			}
			if e = op.Audit(jobCtx, tx, tenant, job.AgentUser, "database.failed", "database", record, registry.NewID(), registry.Row{"error_code": "agent_access_revoked"}); e != nil {
				tx.Rollback()
				return true, e
			}
			if e = tx.Commit(); e != nil {
				return true, e
			}
			return true, nil
		}
	}
	if e == nil && active == registry.StateActive {
		_, e = tx.ExecContext(ctx, "UPDATE database_resources SET state='provisioning',attempts=attempts+1,revision=revision+1 WHERE id=? AND state='pending'", record)
	}
	if e != nil || active != registry.StateActive {
		tx.Rollback()
		if e != nil {
			return true, e
		}
		return true, nil
	}
	if e = tx.Commit(); e != nil {
		return true, e
	}
	attempt, cancel := context.WithTimeout(ctx, 45*time.Second)
	err := p.Provision(attempt, record)
	cancel()
	code, message, state := "", "", "ready"
	if err != nil {
		p.Warn(ctx, "database provisioning failed", err, "resource_id", record, "tenant_id", tenant)
		code, message = ProvisionError(err)
		state = "failed"
	}
	tx, e = db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return true, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE database_resources SET state=?,last_error_code=?,last_error_message=?,last_verified_at=IF(?='ready',UTC_TIMESTAMP(6),last_verified_at),revision=revision+1 WHERE id=? AND state='provisioning'", state, registry.Nullable(code), registry.Nullable(message), state, record); e == nil {
		e = op.Audit(jobCtx, tx, tenant, job.AgentUser, "database."+state, "database", record, registry.NewID(), registry.Row{"error_code": code})
	}
	if e != nil {
		tx.Rollback()
		return true, e
	}
	if e = tx.Commit(); e != nil {
		return true, e
	}
	return true, nil
}

// ProvisionError maps a provisioning failure to the stored error code and message.
func ProvisionError(e error) (string, string) {
	var ae *registry.APIError
	if errors.As(e, &ae) {
		return ae.Code, ae.Message
	}
	var me *mariadb.MySQLError
	if errors.As(e, &me) {
		switch me.Number {
		case 1045:
			return "authentication_failed", "The database rejected the supplied credentials."
		case 1044, 1142, 1227:
			return "insufficient_privileges", "The database account does not have the required privileges."
		case 1049:
			return "database_missing", "The registered database does not exist."
		case 1396:
			return "account_conflict", "The generated account cannot be verified."
		}
	}
	return "connection_or_provisioning_failed", "Could not connect or provision. Check the target, credentials, TLS and network policy, then retry."
}

var (
	// GeneratedName matches managed database and runtime account names.
	GeneratedName = regexp.MustCompile(`^la_[a-f0-9]{32}$`)
	accountHostRE = regexp.MustCompile(`^[a-zA-Z0-9.%:_\-]+$`)
	// generatedPasswordRE matches generated runtime passwords, which are
	// embedded in CREATE USER and must stay quote-free.
	generatedPasswordRE = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)
)

// Provision creates or verifies one resource's database and runtime account.
// A managed resource is serialized by a per-resource lock on its target.
func (p *Provisioner) Provision(ctx context.Context, record string) error {
	job, e := registry.ScanOne[ResourceAccess](ctx, p.DB, "SELECT "+ResourceAccessColumns+" FROM database_resources d JOIN database_targets t ON t.id=d.target_id WHERE d.id=?", record)
	if e != nil {
		return e
	}
	tenant, name := job.TenantID, job.Name
	var runtime Credentials
	if e = p.Sealer.Unseal("database", record, tenant, job.Runtime, &runtime); e != nil {
		return e
	}
	if job.Mode == "registered" {
		db, e := p.Connect(ctx, job.DatabaseEndpoint, runtime, name)
		if e != nil {
			return e
		}
		defer db.Close()
		return ValidateRuntime(ctx, db, name, false)
	}
	principal, marker, e := managedIdentity(record, job, runtime)
	if e != nil {
		return e
	}
	conn, release, e := p.lockManaged(ctx, record, job)
	if e != nil {
		return e
	}
	defer release()
	var comment string
	e = conn.QueryRowContext(ctx, "SELECT SCHEMA_COMMENT FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=?", name).Scan(&comment)
	if errors.Is(e, sql.ErrNoRows) {
		_, e = conn.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci COMMENT='"+marker+"'")
		if e != nil {
			return e
		}
	} else if e != nil {
		return e
	} else if comment != marker {
		return OwnershipConflict()
	}
	_, e = conn.ExecContext(ctx, "CREATE USER "+principal+" IDENTIFIED BY '"+runtime.Password+"'")
	var me *mariadb.MySQLError
	if e != nil && (!errors.As(e, &me) || me.Number != 1396) {
		return e
	}
	verify, e := p.Connect(ctx, job.DatabaseEndpoint, runtime, "")
	if e != nil {
		return OwnershipConflict()
	}
	if e = checkGrants(ctx, verify, name, true, false); e != nil {
		verify.Close()
		return e
	}
	verify.Close()
	pattern := strings.ReplaceAll(name, "_", `\_`)
	if _, e = conn.ExecContext(ctx, "GRANT SELECT,INSERT,UPDATE,DELETE ON `"+pattern+"`.* TO "+principal); e != nil {
		return e
	}
	verify, e = p.Connect(ctx, job.DatabaseEndpoint, runtime, name)
	if e != nil {
		return e
	}
	defer verify.Close()
	return ValidateRuntime(ctx, verify, name, true)
}

func managedIdentity(record string, job ResourceAccess, runtime Credentials) (principal, marker string, err error) {
	if !GeneratedName.MatchString(job.Name) || runtime.Username != job.Name || !accountHostRE.MatchString(runtime.AccountHost) || !generatedPasswordRE.MatchString(runtime.Password) {
		return "", "", OwnershipConflict()
	}
	return "'" + runtime.Username + "'@'" + runtime.AccountHost + "'", "lastapp:" + record + ":" + hex.EncodeToString(job.Ownership), nil
}

func (p *Provisioner) lockManaged(ctx context.Context, record string, job ResourceAccess) (*sql.Conn, func(), error) {
	var admin Credentials
	if e := p.Sealer.Unseal("target", job.TargetID, "", job.Provisioning, &admin); e != nil {
		return nil, nil, e
	}
	db, e := p.Connect(ctx, job.DatabaseEndpoint, admin, "")
	if e != nil {
		return nil, nil, e
	}
	conn, e := db.Conn(ctx)
	if e != nil {
		db.Close()
		return nil, nil, e
	}
	lock := "lastapp:resource:" + record
	var got int
	e = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,5)", lock).Scan(&got)
	if e != nil || got != 1 {
		conn.Close()
		db.Close()
		if e == nil {
			e = registry.Problem(409, "provisioning_busy", "Another worker still owns the resource; retry shortly.")
		}
		return nil, nil, e
	}
	return conn, func() {
		data.ReleasePinnedLock(conn, lock)
		conn.Close()
		db.Close()
	}, nil
}

// ValidateRuntime checks the server version and the runtime account's grants on
// database name; strict rejects anything beyond SELECT, INSERT, UPDATE, DELETE.
func ValidateRuntime(ctx context.Context, db *sql.DB, name string, strict bool) error {
	var version string
	if e := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); e != nil {
		return e
	}
	if !strings.Contains(version, "MariaDB") {
		return registry.Problem(400, "unsupported_database", "This release supports MariaDB only.")
	}
	parts := strings.Split(version, ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if major < 10 || (major == 10 && minor < 11) {
		return registry.Problem(400, "unsupported_version", "MariaDB 10.11 or newer is required.")
	}
	return checkGrants(ctx, db, name, strict, true)
}

func checkGrants(ctx context.Context, db *sql.DB, name string, strict, required bool) error {
	rs, e := db.QueryContext(ctx, "SHOW GRANTS")
	if e != nil {
		return e
	}
	defer rs.Close()
	have := map[string]bool{}
	scope := "`" + strings.ReplaceAll(name, "_", `\_`) + "`.*"

	for rs.Next() {
		var grant string
		if e = rs.Scan(&grant); e != nil {
			return e
		}
		if strings.Contains(grant, "WITH GRANT OPTION") {
			return registry.Problem(400, "unsafe_grants", "Runtime credentials must not have GRANT OPTION.")
		}
		if strings.HasPrefix(grant, "GRANT USAGE ON *.* TO ") {
			continue
		}
		on, to := strings.Index(grant, " ON "), strings.Index(grant, " TO ")
		if on < 0 || to < on {
			return registry.Problem(400, "unsupported_grants", "Use explicit schema grants, rather than database roles, for runtime accounts.")
		}
		privs := strings.TrimPrefix(grant[:on], "GRANT ")
		target := grant[on+4 : to]
		if target != scope {
			return registry.Problem(400, "unsafe_grants", "Runtime credentials must be restricted to this database.")
		}
		if privs == "ALL PRIVILEGES" && !strict {
			for _, p := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
				have[p] = true
			}
			continue
		}
		for _, v := range strings.Split(privs, ", ") {
			switch v {
			case "SELECT", "INSERT", "UPDATE", "DELETE":
				have[v] = true
			default:
				if strict {
					return OwnershipConflict()
				}
			}
		}
	}
	if e = rs.Err(); e != nil {
		return e
	}
	if required {
		for _, v := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			if !have[v] {
				return registry.Problem(400, "insufficient_privileges", "Runtime credentials require SELECT, INSERT, UPDATE and DELETE on the complete database.")
			}
		}
	}
	return nil
}

// ProvisioningJob is a pending database resource and the delegation, if any,
// that requested it.
type ProvisioningJob struct {
	ID, TenantID, TargetID             string
	AgentGrant, AgentUser, AgentClient string // empty for a console request
}

func (v *ProvisioningJob) ScanFields() []any {
	return []any{&v.ID, &v.TenantID, &v.TargetID, &v.AgentGrant, &v.AgentUser, &v.AgentClient}
}

// NextJob round-robins Tenants, oldest job within each Tenant. A slow Tenant can
// consume at most one 45-second attempt before another waiting Tenant gets a turn.
func NextJob(ctx context.Context, q registry.Queryer, lastTenant string) (ProvisioningJob, error) {
	return registry.ScanOne[ProvisioningJob](ctx, q, "SELECT d.id,d.tenant_id,d.target_id,COALESCE(d.agent_grant_id,''),COALESCE(d.agent_user_id,''),COALESCE(ag.client_id,'') FROM database_resources d JOIN tenants t ON t.id=d.tenant_id JOIN database_targets dt ON dt.id=d.target_id LEFT JOIN agent_grants ag ON ag.id=d.agent_grant_id WHERE d.state='pending' AND d.preview_cleanup_at IS NULL AND t.state='active' AND dt.state='active' ORDER BY (d.tenant_id>?) DESC,d.tenant_id,d.created_at,d.id LIMIT 1", lastTenant)
}
