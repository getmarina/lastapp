// Package databases owns database targets and resources: registration,
// allocation for Spaces and the credential records the provisioning worker uses.
package databases

import (
	"context"
	"crypto/rand"
	"database/sql"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/config"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

type Credentials struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	AccountHost string `json:"account_host"`
}

type DatabaseInput struct {
	ID             string      `json:"id"`
	Mode           string      `json:"management_mode"`
	Layout         string      `json:"layout"`
	Target         string      `json:"target_id"`
	Database       string      `json:"database_name"`
	Credentials    Credentials `json:"credentials"`
	PackagePreview bool        `json:"-"`
}

// Target is a database server registered by the platform.
type Target struct {
	Host      string       `json:"canonical_host"`
	CreatedAt time.Time    `json:"created_at"`
	Name      string       `json:"display_name"`
	ID        string       `json:"id"`
	Managed   registry.Bit `json:"managed_enabled"`
	Port      int64        `json:"port"`
	Revision  int64        `json:"revision"`
	State     string       `json:"state"`
	TLS       string       `json:"tls_mode"`
}

func (v *Target) ScanFields() []any {
	return []any{&v.Host, &v.CreatedAt, &v.Name, &v.ID, &v.Managed, &v.Port, &v.Revision, &v.State, &v.TLS}
}

// AvailableTarget is an active target a Tenant may allocate on.
type AvailableTarget struct {
	Name    string       `json:"display_name"`
	ID      string       `json:"id"`
	Managed registry.Bit `json:"managed_enabled"`
}

func (v *AvailableTarget) ScanFields() []any { return []any{&v.Name, &v.ID, &v.Managed} }

// Database is a Tenant database resource and its provisioning status.
type Database struct {
	Attempts     int64      `json:"attempts"`
	CreatedAt    time.Time  `json:"created_at"`
	Name         string     `json:"database_name"`
	ID           string     `json:"id"`
	LastError    *string    `json:"last_error_code"`
	LastMessage  *string    `json:"last_error_message"`
	LastVerified *time.Time `json:"last_verified_at"`
	Layout       string     `json:"layout"`
	Mode         string     `json:"management_mode"`
	Revision     int64      `json:"revision"`
	State        string     `json:"state"`
	TargetID     string     `json:"target_id"`
	TenantID     string     `json:"tenant_id"`
}

const databaseColumns = "attempts,created_at,database_name,id,last_error_code,last_error_message,last_verified_at,layout,management_mode,revision,state,target_id,tenant_id"

func (v *Database) ScanFields() []any {
	return []any{&v.Attempts, &v.CreatedAt, &v.Name, &v.ID, &v.LastError, &v.LastMessage, &v.LastVerified, &v.Layout, &v.Mode, &v.Revision, &v.State, &v.TargetID, &v.TenantID}
}

// DatabaseEndpoint is where a target is reached and how its TLS is checked.
type DatabaseEndpoint struct {
	Host string
	Port int
	TLS  string
}

// ResourceAccess is a database resource with its target's endpoint and the
// sealed credentials needed to connect to it.
type ResourceAccess struct {
	DatabaseEndpoint
	TenantID, TargetID, Name, Mode string
	Runtime, Schema, Ownership     []byte
	Provisioning                   []byte // the target's, for managed resources
}

const ResourceAccessColumns = "d.tenant_id,d.target_id,d.database_name,d.management_mode,d.runtime_credentials_enc,d.schema_credentials_enc,d.ownership_token,t.canonical_host,t.port,t.tls_mode,t.provisioning_credentials_enc"

func (v *ResourceAccess) ScanFields() []any {
	return []any{&v.TenantID, &v.TargetID, &v.Name, &v.Mode, &v.Runtime, &v.Schema, &v.Ownership, &v.Host, &v.Port, &v.TLS, &v.Provisioning}
}

// TargetEndpoint reads a target's endpoint.
func TargetEndpoint(ctx context.Context, q registry.Queryer, target string) (DatabaseEndpoint, error) {
	var v DatabaseEndpoint
	return v, registry.Lookup(ctx, q, "SELECT canonical_host,port,tls_mode FROM database_targets WHERE id=?", []any{target}, &v.Host, &v.Port, &v.TLS)
}

// Storage owns database targets and Tenant database resources. Console
// handlers and MCP tools both call these methods; the worker applies the
// queued work and rechecks its authorizer separately.
type Storage struct {
	DB           *sql.DB
	Run          *op.Runner
	Seal         func(purpose, record, tenant string, value any) ([]byte, error)
	Config       *config.Config
	RegistryName string
}

type TargetPatch struct {
	Name        *string      `json:"display_name"`
	State       *string      `json:"state"`
	Managed     *bool        `json:"managed_enabled"`
	Credentials *Credentials `json:"credentials"`
}

// DatabaseChange is a registered credential replacement or retirement; a
// retry carries neither.
type DatabaseChange struct {
	State       string       `json:"state"`
	Credentials *Credentials `json:"credentials"`
}

func (v *Storage) Targets(ctx context.Context, a op.Actor, page op.PageRequest) (op.Page[Target], error) {
	if e := v.Run.Authorize(ctx, a, "platform.admin", access.Resource{}); e != nil {
		return op.Page[Target]{}, e
	}
	return op.PageScan[Target](ctx, v.DB, page, op.CollectionFor("database-targets"), "SELECT canonical_host,created_at,display_name,id,managed_enabled,port,revision,state,tls_mode FROM database_targets ORDER BY display_name")
}

// AvailableTargets lists the active targets a Tenant may allocate on.
func (v *Storage) AvailableTargets(ctx context.Context, a op.Actor, tenant string, managed bool, page op.PageRequest) (op.Page[AvailableTarget], error) {
	if e := v.Run.Authorize(ctx, a, "database.manage", access.Resource{Tenant: tenant}); e != nil {
		return op.Page[AvailableTarget]{}, e
	}
	filter := ""
	if managed {
		filter = " AND managed_enabled=1"
	}
	return op.PageScan[AvailableTarget](ctx, v.DB, page, op.CollectionFor("database-targets"), "SELECT display_name,id,managed_enabled FROM database_targets WHERE state='active'"+filter+" ORDER BY display_name")
}

type TargetInput struct {
	ID          string       `json:"id"`
	Name        string       `json:"display_name"`
	Host        string       `json:"canonical_host"`
	Port        int          `json:"port"`
	TLS         string       `json:"tls_mode"`
	State       string       `json:"state"`
	Managed     bool         `json:"managed_enabled"`
	Credentials *Credentials `json:"credentials"`
}

var (
	hostRE         = regexp.MustCompile(`^[a-zA-Z0-9.:\-]+$`)
	databaseNameRE = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
)

func (v *Storage) validateTarget(in *TargetInput) error {
	in.Host = strings.ToLower(strings.TrimSuffix(in.Host, "."))
	if !hostRE.MatchString(in.Host) || len(in.Host) > 253 || in.Port < 1 || in.Port > 65535 {
		return registry.Bad("Invalid database endpoint.")
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 160 {
		return registry.Bad("Invalid target name.")
	}
	if in.TLS != "verify_identity" && !(in.TLS == "disabled" && v.Config.Development) {
		return registry.Bad("TLS must verify the server identity; disabled is allowed only in local development.")
	}
	if in.State == "" {
		in.State = registry.StateActive
	}
	if !registry.EnabledState(in.State) {
		return registry.Bad("Invalid target state.")
	}
	return nil
}

func (v *Storage) CreateTarget(ctx context.Context, a op.Actor, in TargetInput) (op.Ref, error) {
	if e := registry.ID(in.ID); e != nil {
		return op.Ref{}, e
	}
	if e := v.validateTarget(&in); e != nil {
		return op.Ref{}, e
	}
	if in.Managed && (in.Credentials == nil || in.Credentials.Username == "" || in.Credentials.Password == "") {
		return op.Ref{}, registry.Bad("Managed targets require provisioning credentials.")
	}
	e := v.Run.Mutate(ctx, a, "platform.admin", access.Resource{}, func(tx *sql.Tx) error {
		var enc any
		if in.Credentials != nil {
			b, e := v.Seal("target", in.ID, "", in.Credentials)
			if e != nil {
				return e
			}
			enc = b
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO database_targets (id,display_name,canonical_host,port,tls_mode,state,managed_enabled,provisioning_credentials_enc) VALUES (?,?,?,?,?,?,?,?)", in.ID, in.Name, in.Host, in.Port, in.TLS, in.State, in.Managed, enc)
		if e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, a, "", "target.created", "database_target", in.ID, registry.Row{})
	})
	if e != nil {
		return op.Ref{}, e
	}
	return v.Run.Ref(ctx, "database_targets", "", in.ID)
}

func (v *Storage) UpdateTarget(ctx context.Context, a op.Actor, record string, in TargetPatch, revision int64) (op.Ref, error) {
	if revision < 1 {
		return op.Ref{}, op.RevisionRequired()
	}
	e := v.Run.Mutate(ctx, a, "platform.admin", access.Resource{}, func(tx *sql.Tx) error {
		old, e := registry.LockRecord(ctx, tx, "database_targets", "", record)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(old.Revision, revision); e != nil {
			return e
		}
		sets := []string{"revision=revision+1"}
		args := []any{}
		if in.Name != nil {
			if strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 160 {
				return registry.Bad("Invalid name.")
			}
			sets = append(sets, "display_name=?")
			args = append(args, *in.Name)
		}
		if in.State != nil {
			if !registry.EnabledState(*in.State) {
				return registry.Bad("Invalid state.")
			}
			sets = append(sets, "state=?")
			args = append(args, *in.State)
		}
		if in.Managed != nil {
			sets = append(sets, "managed_enabled=?")
			args = append(args, *in.Managed)
		}
		if in.Credentials != nil {
			if in.Credentials.Username == "" || in.Credentials.Password == "" {
				return registry.Bad("Credentials cannot be empty.")
			}
			b, e := v.Seal("target", record, "", in.Credentials)
			if e != nil {
				return e
			}
			sets = append(sets, "provisioning_credentials_enc=?")
			args = append(args, b)
		}
		args = append(args, record)
		if _, e = tx.ExecContext(ctx, "UPDATE database_targets SET "+strings.Join(sets, ",")+" WHERE id=?", args...); e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, a, "", "target.updated", "database_target", record, registry.Row{})
	})
	if e != nil {
		return op.Ref{}, e
	}
	return v.Run.Ref(ctx, "database_targets", "", record)
}

func (v *Storage) Databases(ctx context.Context, a op.Actor, tenant string, page op.PageRequest) (op.Page[Database], error) {
	if e := v.Run.Authorize(ctx, a, "database.read", access.Resource{Tenant: tenant}); e != nil {
		return op.Page[Database]{}, e
	}
	return op.PageScan[Database](ctx, v.DB, page, op.CollectionFor("database-resources"), "SELECT "+databaseColumns+" FROM database_resources WHERE tenant_id=? ORDER BY created_at DESC", tenant)
}

func (v *Storage) Database(ctx context.Context, a op.Actor, tenant, record string) (Database, error) {
	if e := v.Run.Authorize(ctx, a, "database.read", access.Resource{Tenant: tenant}); e != nil {
		return Database{}, e
	}
	return registry.ScanOne[Database](ctx, v.DB, "SELECT "+databaseColumns+" FROM database_resources WHERE tenant_id=? AND id=?", tenant, record)
}

// The Tenant mutation lock serializes allocation counts, including retries that
// claim an existing resource. Retrying the same resource does not use a new slot.
func agentDatabaseProvision(ctx context.Context, tx *sql.Tx, p access.Principal, resource, mode, target string) error {
	if p.Agent == nil {
		return nil
	}
	g := p.Agent
	if !access.AgentHas(g, "databases.provision") || mode != "managed" || !slices.Contains(g.Limits.TargetIDs, target) {
		return registry.Problem(403, "needs_authorization", "Managed provisioning requires an approved target and databases.provision.")
	}
	var allocated int
	if e := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM database_resources WHERE agent_grant_id=? AND id<>?", g.ID, resource).Scan(&allocated); e != nil {
		return e
	}
	if allocated >= g.Limits.MaxDatabases {
		return registry.Problem(403, "agent_limit", "This agent has reached its database allocation limit.")
	}
	return nil
}

// Allocate requests a database resource inside the caller's transaction. A
// workspace allocation is authorized by workspace.manage on its scope and is
// not counted against an agent's database allocation.
func (v *Storage) Allocate(ctx context.Context, tx *sql.Tx, a op.Actor, tenant string, in DatabaseInput, policy registry.Policy, workspace *access.Resource) error {
	action, res := "database.manage", access.Resource{Tenant: tenant}
	if workspace != nil {
		action, res = "workspace.manage", *workspace
	}
	if e := v.Run.AuthorizeTx(ctx, tx, a, action, res); e != nil {
		return e
	}
	if e := registry.ID(in.ID); e != nil {
		return e
	}
	if e := registry.ID(in.Target); e != nil {
		return e
	}
	p := a.Principal
	if workspace == nil {
		if e := agentDatabaseProvision(ctx, tx, p, in.ID, in.Mode, in.Target); e != nil {
			return e
		}
	}
	if e := v.CheckAllocation(ctx, tx, tenant, in, policy, workspace); e != nil {
		return e
	}
	var ownership any
	cred := in.Credentials
	if in.Mode == "managed" {
		in.Database = "la_" + strings.ReplaceAll(in.ID, "-", "")
		cred = Credentials{Username: in.Database, Password: registry.Token(32), AccountHost: v.Config.RuntimeAccountHost}
		b := make([]byte, 16)
		if _, e := rand.Read(b); e != nil {
			return e
		}
		ownership = b
	} else {
		if !databaseNameRE.MatchString(in.Database) || strings.EqualFold(in.Database, v.RegistryName) {
			return registry.Bad("Invalid or protected database name.")
		}
		if cred.Username == "" || cred.Password == "" {
			return registry.Bad("Existing database credentials are required.")
		}
	}
	enc, e := v.Seal("database", in.ID, tenant, cred)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO database_resources (id,tenant_id,target_id,database_name,management_mode,layout,runtime_credentials_enc,ownership_token,package_preview) VALUES (?,?,?,?,?,?,?,?,?)", in.ID, tenant, in.Target, in.Database, in.Mode, in.Layout, enc, ownership, in.PackagePreview)
	if e != nil {
		return e
	}
	if p.Agent != nil {
		if _, e = tx.ExecContext(ctx, "UPDATE database_resources SET agent_grant_id=?,agent_user_id=? WHERE id=?", p.Agent.ID, p.ID, in.ID); e != nil {
			return e
		}
	}
	return op.AuditAs(ctx, tx, a, tenant, "database.requested", "database", in.ID, registry.Row{"management_mode": in.Mode, "layout": in.Layout})
}

// CheckAllocation checks storage capacity and target policy during planning and
// repeats the same checks under the allocation transaction's Tenant lock.
func (v *Storage) CheckAllocation(ctx context.Context, q registry.Queryer, tenant string, in DatabaseInput, policy registry.Policy, workspace *access.Resource) error {
	if e := registry.LayoutAllowed(policy.Plan, in.Layout); e != nil {
		return e
	}
	if in.Mode != "managed" && in.Mode != "registered" {
		return registry.Bad("Select managed or registered database mode.")
	}
	if !policy.AllowsMethod(in.Mode) {
		return registry.Problem(403, "plan_restriction", "This database method is not included in the plan.")
	}
	if e := allocationQuota(ctx, q, tenant, in, policy, workspace); e != nil {
		return e
	}
	if e := pendingDatabaseQuota(ctx, q, tenant); e != nil {
		return e
	}
	var targetState string
	var managed registry.Bit
	e := registry.Lookup(ctx, q, "SELECT state,managed_enabled FROM database_targets WHERE id=?", []any{in.Target}, &targetState, &managed)
	if e != nil {
		return e
	}
	if targetState != registry.StateActive {
		return registry.Conflict("The database target is disabled.")
	}
	if in.Mode == "managed" {
		if !managed {
			return registry.Bad("Target does not support managed creation.")
		}
		if v.Config.RuntimeAccountHost == "" {
			return registry.Bad("Runtime database account host is not configured.")
		}
	}
	return nil
}

func allocationQuota(ctx context.Context, q registry.Queryer, tenant string, in DatabaseInput, policy registry.Policy, workspace *access.Resource) error {
	if !in.PackagePreview {
		return registry.Quota(ctx, q, tenant, "database_resources", policy.MaxDatabases)
	}
	if workspace == nil || workspace.Tenant != tenant || workspace.Space == "" || in.Mode != "managed" {
		return registry.Bad("Package preview storage requires a managed workspace database.")
	}
	var n int
	if e := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM development_workspaces w
		 JOIN spaces sp ON sp.id=w.backing_space_id AND sp.tenant_id=w.tenant_id
		 JOIN database_resources d ON d.id=sp.database_resource_id AND d.tenant_id=w.tenant_id
		 WHERE w.tenant_id=? AND w.production_space_id=? AND d.package_preview=1 AND d.state<>'retired'`, tenant, workspace.Space).Scan(&n); e != nil {
		return e
	}
	if n >= registry.MaxSpaceWorkspaces {
		return registry.Conflict("The Space's package preview storage is full. Archive unused package workspaces and wait for cleanup.")
	}
	return nil
}

// CreateDatabase queues a managed allocation or registers an existing database.
func (v *Storage) CreateDatabase(ctx context.Context, a op.Actor, tenant string, in DatabaseInput) (op.Ref, error) {
	e := v.Run.Mutate(ctx, a, "database.manage", access.Resource{Tenant: tenant}, func(tx *sql.Tx) error {
		policy, e := registry.PolicyFor(ctx, tx, tenant)
		if e != nil {
			return e
		}
		return v.Allocate(ctx, tx, a, tenant, in, policy, nil)
	})
	if e != nil {
		return op.Ref{}, e
	}
	return v.Run.Ref(ctx, "database_resources", tenant, in.ID)
}

// RetryDatabase requeues a failed or ready resource under its revision.
func (v *Storage) RetryDatabase(ctx context.Context, a op.Actor, tenant, record string, revision int64) (op.Ref, error) {
	return v.changeDatabase(ctx, a, tenant, record, DatabaseChange{}, true, revision)
}

// UpdateDatabase retires a resource or replaces registered credentials.
func (v *Storage) UpdateDatabase(ctx context.Context, a op.Actor, tenant, record string, in DatabaseChange, revision int64) (op.Ref, error) {
	return v.changeDatabase(ctx, a, tenant, record, in, false, revision)
}

func (v *Storage) changeDatabase(ctx context.Context, a op.Actor, tenant, record string, in DatabaseChange, retry bool, revision int64) (op.Ref, error) {
	if revision < 1 {
		return op.Ref{}, op.RevisionRequired()
	}
	p := a.Principal
	e := v.Run.Mutate(ctx, a, "database.manage", access.Resource{Tenant: tenant}, func(tx *sql.Tx) error {
		var current int64
		var state, mode, target string
		var cleaning bool
		e := registry.Lookup(ctx, tx, "SELECT revision,state,management_mode,target_id,preview_cleanup_at IS NOT NULL FROM database_resources WHERE tenant_id=? AND id=? FOR UPDATE", []any{tenant, record}, &current, &state, &mode, &target, &cleaning)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(current, revision); e != nil {
			return e
		}
		if cleaning {
			return registry.Conflict("The archived package preview database is being reclaimed.")
		}
		if state == "provisioning" || state == "pending" || state == registry.StateRetired {
			return registry.Conflict("Wait for provisioning to finish; retired resources cannot be changed.")
		}
		next := "pending"
		if !retry && in.Credentials == nil {
			if in.State != registry.StateRetired {
				return registry.Bad("Only retirement or registered credential replacement is supported.")
			}
			var n int
			if e = tx.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM spaces WHERE database_resource_id=?)+(SELECT COUNT(*) FROM tenants WHERE default_shared_database_id=?)", record, record).Scan(&n); e != nil {
				return e
			}
			if n > 0 {
				return registry.Conflict("A bound or default database cannot be retired.")
			}
			next = registry.StateRetired
		}
		if next == "pending" {
			if e := agentDatabaseProvision(ctx, tx, p, record, mode, target); e != nil {
				return e
			}
			if e := pendingDatabaseQuota(ctx, tx, tenant); e != nil {
				return e
			}
		}
		if in.Credentials != nil {
			if mode != "registered" || in.Credentials.Username == "" || in.Credentials.Password == "" {
				return registry.Bad("Only registered database credentials can be replaced.")
			}
			enc, e := v.Seal("database", record, tenant, in.Credentials)
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "UPDATE database_resources SET runtime_credentials_enc=?,credential_generation=credential_generation+1 WHERE id=?", enc, record); e != nil {
				return e
			}
		}
		if next == "pending" {
			var grant, user any
			if p.Agent != nil {
				grant, user = p.Agent.ID, p.ID
			}
			// Requeued work belongs to its current authorizer. A human retry
			// explicitly takes ownership from any old, possibly revoked grant.
			if _, e = tx.ExecContext(ctx, "UPDATE database_resources SET agent_grant_id=?,agent_user_id=? WHERE id=?", grant, user, record); e != nil {
				return e
			}
		}
		_, e = tx.ExecContext(ctx, "UPDATE database_resources SET state=?,last_error_code=NULL,last_error_message=NULL,revision=revision+1 WHERE id=?", next, record)
		if e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, a, tenant, "database."+next, "database", record, registry.Row{})
	})
	if e != nil {
		return op.Ref{}, e
	}
	return v.Run.Ref(ctx, "database_resources", tenant, record)
}

func pendingDatabaseQuota(ctx context.Context, q registry.Queryer, tenant string) error {
	var pending int
	if e := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_resources
		 WHERE tenant_id=? AND (state='provisioning' OR (state='pending' AND preview_cleanup_at IS NULL))`, tenant).Scan(&pending); e != nil {
		return e
	}
	if pending >= 8 {
		return registry.Problem(409, "pending_limit", "Wait for pending database requests before queuing more.")
	}
	return nil
}

// OwnershipConflict is the error for an existing database object or account
// that Lastapp did not create; nothing is replaced.
func OwnershipConflict() error {
	return registry.Problem(409, "ownership_conflict", "Existing database or account does not match Lastapp ownership. Nothing was replaced.")
}
