package registry

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Plan is a subscription plan: allowed database layouts and methods, compute
// and the Tenant quotas (nil is unlimited).
type Plan struct {
	AllowDedicatedCompute  Bit       `json:"allow_dedicated_compute"`
	AllowDedicatedDatabase Bit       `json:"allow_dedicated_database"`
	AllowManaged           Bit       `json:"allow_managed_database"`
	AllowRegistered        Bit       `json:"allow_registered_database"`
	AllowSharedDatabase    Bit       `json:"allow_shared_database"`
	Code                   string    `json:"code"`
	CreatedAt              time.Time `json:"created_at"`
	DefaultLayout          string    `json:"default_layout"`
	Name                   string    `json:"display_name"`
	ID                     string    `json:"id"`
	MaxApps                *int64    `json:"max_apps"`
	MaxDatabases           *int64    `json:"max_databases"`
	MaxSpaces              *int64    `json:"max_spaces"`
	Revision               int64     `json:"revision"`
}

const PlanColumns = "allow_dedicated_compute,allow_dedicated_database,allow_managed_database,allow_registered_database,allow_shared_database,code,created_at,default_layout,display_name,id,max_apps,max_databases,max_spaces,revision"

func (p *Plan) ScanFields() []any {
	return []any{&p.AllowDedicatedCompute, &p.AllowDedicatedDatabase, &p.AllowManaged, &p.AllowRegistered, &p.AllowSharedDatabase, &p.Code, &p.CreatedAt, &p.DefaultLayout, &p.Name, &p.ID, &p.MaxApps, &p.MaxDatabases, &p.MaxSpaces, &p.Revision}
}

// Policy is the plan of an active Tenant with the Tenant's own defaults.
type Policy struct {
	Plan
	TenantState           string
	TenantLayout          string // empty when the Tenant uses the plan default
	DefaultSharedDatabase string // empty until the first shared database exists
}

func (p *Policy) ScanFields() []any {
	return append(p.Plan.ScanFields(), &p.TenantState, &p.TenantLayout, &p.DefaultSharedDatabase)
}

// allowsLayout reports whether the plan permits a database layout.
func (p Plan) allowsLayout(layout string) bool {
	return layout == "shared" && bool(p.AllowSharedDatabase) || layout == "dedicated" && bool(p.AllowDedicatedDatabase)
}

// AllowsMethod reports whether the plan permits a database provisioning mode.
func (p Plan) AllowsMethod(mode string) bool {
	return mode == "managed" && bool(p.AllowManaged) || mode == "registered" && bool(p.AllowRegistered)
}

func LayoutAllowed(policy Plan, layout string) error {
	if layout == "" || policy.allowsLayout(layout) {
		return nil
	}
	return Problem(403, "plan_restriction", "This database layout is not permitted by the selected plan.")
}

// PolicyFor reads the plan of an active Tenant; resources are allocated only
// in an active Tenant.
func PolicyFor(ctx context.Context, q Queryer, tenant string) (Policy, error) {
	v, e := ScanOne[Policy](ctx, q, "SELECT "+Prefixed("p", PlanColumns)+",t.state,COALESCE(t.default_layout,''),COALESCE(t.default_shared_database_id,'') FROM plans p JOIN tenants t ON t.plan_id=p.id WHERE t.id=?", tenant)
	if e != nil {
		return Policy{}, e
	}
	if v.TenantState != StateActive {
		return Policy{}, Conflict("The Tenant must be active to allocate resources.")
	}
	return v, nil
}

// quotaNames names each quota-limited table in the quota_exceeded message.
var quotaNames = map[string]string{"spaces": "spaces", "apps": "apps", "database_resources": "databases"}

// Quota rejects one more production record of table when limit (nil is
// unlimited) is reached.
func Quota(ctx context.Context, q Queryer, tenant, table string, limit *int64) error {
	if limit == nil {
		return nil
	}
	var n int
	state := "state<>'archived'"
	if table == "spaces" {
		state += " AND workspace_id IS NULL"
	}
	if table == "apps" {
		state += " AND NOT EXISTS(SELECT 1 FROM spaces sp WHERE sp.id=apps.space_id AND sp.workspace_id IS NOT NULL)"
	}
	if table == "database_resources" {
		state = "package_preview=0"
	}
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=? AND "+state, tenant).Scan(&n); e != nil {
		return e
	}
	if int64(n) >= *limit {
		return Problem(409, "quota_exceeded", "The Tenant's "+quotaNames[table]+" limit has been reached.")
	}
	return nil
}

// Administrative metadata has deliberately generous but finite limits. These are
// safety ceilings, independent of a subscription's usually smaller resource quotas.
var MetadataLimits = map[string]int{
	"tenant_memberships": 1000, "groups": 200, "group_memberships": 10000,
	"roles": 2000, "access_grants": 10000, "microapp_installations": 1000,
}

func MetadataQuota(ctx context.Context, q Queryer, tenant, table string, additional ...int) error {
	required := 1
	if len(additional) > 0 {
		required = additional[0]
	}
	var n int
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=?", tenant).Scan(&n); e != nil {
		return e
	}
	if n+required > MetadataLimits[table] {
		return Problem(409, "metadata_limit", "The Tenant's administrative metadata limit has been reached.")
	}
	return nil
}

const (
	MaxWorkspaceApps       = 100
	MaxOwnedWorkspaces     = 5  // active workspaces per user per production Space
	MaxSpaceWorkspaces     = 20 // active workspaces per production Space
	MaxReleaseMicroapps    = 100
	MaxReleaseDependencies = 100
	MaxSpaceSnapshotBytes  = 1 << 20
	MaxAppBuilds           = 100
	MaxAppReleases         = 100
	MaxOutstandingUploads  = 10
	UploadTicketLifetime   = 10 * time.Minute
	// LaunchLifetime bounds a single-use App launch code: console launches
	// (internal/dataapi) and agent development launches (internal/delivery).
	LaunchLifetime = 2 * time.Minute
)

// Workspace Apps consume only the workspace's metadata capacity. Production
// subscription limits apply to production creation/restoration and deployment.
func AppQuota(ctx context.Context, q Queryer, tenant, space string, policy Policy, creating bool) error {
	var workspace sql.NullString
	if e := q.QueryRowContext(ctx, "SELECT workspace_id FROM spaces WHERE tenant_id=? AND id=?", tenant, space).Scan(&workspace); e != nil {
		return e
	}
	if !workspace.Valid {
		return Quota(ctx, q, tenant, "apps", policy.MaxApps)
	}
	if !creating {
		return nil // Restoring an existing App does not allocate another slot.
	}
	var count int
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM apps WHERE space_id=?", space).Scan(&count); e != nil {
		return e
	}
	if count >= MaxWorkspaceApps {
		return Conflict("Workspace App limit reached.")
	}
	return nil
}

func ActiveTenantUser(ctx context.Context, q Queryer, t, user string) error {
	var state string
	e := q.QueryRowContext(ctx, "SELECT u.state FROM users u JOIN tenant_memberships m ON m.user_id=u.id AND m.tenant_id=? AND m.state='active' WHERE u.id=? FOR UPDATE", t, user).Scan(&state)
	if errors.Is(e, sql.ErrNoRows) {
		return Bad("An active Tenant member is required.")
	}
	if e != nil {
		return e
	}
	if state != StateActive {
		return Bad("An active Tenant member is required.")
	}
	return nil
}
