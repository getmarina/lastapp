package databases

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	mariadb "github.com/aantich/go-mariadb"

	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

// SchedulePreviewCleanup retains archive history while relinquishing the
// managed test database. The Tenant lock also covers provisioning admission.
func SchedulePreviewCleanup(ctx context.Context, tx *sql.Tx, workspace string) error {
	_, e := tx.ExecContext(ctx, `UPDATE database_resources d
		 JOIN spaces sp ON sp.database_resource_id=d.id AND sp.tenant_id=d.tenant_id
		 JOIN development_workspaces w ON w.backing_space_id=sp.id AND w.id=sp.workspace_id
		 SET d.preview_cleanup_at=UTC_TIMESTAMP(6),d.revision=d.revision+1
		 WHERE w.id=? AND w.state='archived' AND sp.state='archived' AND d.package_preview=1
		 AND d.preview_cleanup_at IS NULL AND d.state<>'retired'`, workspace)
	return e
}

// PreviewCleanupJob reclaims one scheduled database, keeping failed cleanup
// charged against preview capacity until it actually succeeds.
func (p *Provisioner) PreviewCleanupJob(ctx context.Context, q registry.Queryer, after string) (tenant string, progressed bool, err error) {
	var record string
	var attempts int
	e := registry.Lookup(ctx, q, `SELECT d.tenant_id,d.id,d.attempts FROM database_resources d
		 JOIN database_targets t ON t.id=d.target_id
		 WHERE d.package_preview=1 AND d.preview_cleanup_at<=UTC_TIMESTAMP(6)
		 AND d.state NOT IN ('provisioning','retired') AND d.tenant_id>?
		 AND (d.attempts=0 OR t.state='active')
		 ORDER BY d.tenant_id,d.preview_cleanup_at,d.id LIMIT 1`, []any{after}, &tenant, &record, &attempts)
	if e != nil {
		var ae *registry.APIError
		if errors.As(e, &ae) && ae.Status == 404 {
			return "", false, nil
		}
		return tenant, false, e
	}
	if attempts != 0 {
		attempt, cancel := context.WithTimeout(ctx, 45*time.Second)
		e = p.removePreview(attempt, record)
		cancel()
	}
	if e != nil {
		if _, updateErr := q.ExecContext(ctx, `UPDATE database_resources
			 SET preview_cleanup_at=UTC_TIMESTAMP(6)+INTERVAL 1 MINUTE
			 WHERE id=? AND preview_cleanup_at IS NOT NULL`, record); updateErr != nil {
			return tenant, false, updateErr
		}
		return tenant, false, e
	}
	tx, e := p.DB.BeginTx(ctx, nil)
	if e != nil {
		return tenant, false, e
	}
	defer tx.Rollback()
	if e = registry.LockTenant(ctx, tx, tenant); e != nil {
		return tenant, false, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE database_resources
		 SET state='retired',preview_cleanup_at=NULL,last_error_code=NULL,last_error_message=NULL,revision=revision+1
		 WHERE id=? AND package_preview=1 AND preview_cleanup_at IS NOT NULL`, record); e != nil {
		return tenant, false, e
	}
	if e = op.Audit(ctx, tx, tenant, "", "database.retired", "database", record, registry.NewID(), registry.Row{"package_preview": true}); e != nil {
		return tenant, false, e
	}
	e = tx.Commit()
	return tenant, e == nil, e
}

func (p *Provisioner) removePreview(ctx context.Context, record string) error {
	job, e := registry.ScanOne[ResourceAccess](ctx, p.DB, "SELECT "+ResourceAccessColumns+` FROM database_resources d
		 JOIN database_targets t ON t.id=d.target_id AND t.state='active' WHERE d.id=? AND d.package_preview=1 AND d.preview_cleanup_at IS NOT NULL
		 AND d.management_mode='managed' AND d.state NOT IN ('provisioning','retired')
		 AND NOT EXISTS(SELECT 1 FROM spaces sp WHERE sp.database_resource_id=d.id AND (sp.workspace_id IS NULL OR sp.state<>'archived'))
		 AND NOT EXISTS(SELECT 1 FROM tenants te WHERE te.default_shared_database_id=d.id)`, record)
	if e != nil {
		return e
	}
	if job.Name != "la_"+strings.ReplaceAll(record, "-", "") || len(job.Ownership) != 16 {
		return OwnershipConflict()
	}
	var runtime Credentials
	if e = p.Sealer.Unseal("database", record, job.TenantID, job.Runtime, &runtime); e != nil {
		return e
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
	e = conn.QueryRowContext(ctx, "SELECT SCHEMA_COMMENT FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=?", job.Name).Scan(&comment)
	if e == nil && comment != marker {
		return OwnershipConflict()
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	// Drop the verified account first so retries can complete after either DDL.
	var statement string
	e = conn.QueryRowContext(ctx, "SHOW CREATE USER "+principal).Scan(&statement)
	if e == nil {
		verify, e := p.Connect(ctx, job.DatabaseEndpoint, runtime, "")
		if e != nil {
			return OwnershipConflict()
		}
		e = checkGrants(ctx, verify, job.Name, true, false)
		verify.Close()
		if e != nil {
			return e
		}
		if _, e = conn.ExecContext(ctx, "DROP USER "+principal); e != nil {
			return e
		}
	} else {
		var me *mariadb.MySQLError
		if !errors.As(e, &me) || me.Number != 1396 {
			return e
		}
	}
	_, e = conn.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+job.Name+"`")
	return e
}
