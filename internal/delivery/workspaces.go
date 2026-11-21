package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/databases"
	"github.com/aantich/lastapp/internal/entities"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/roles"
)

type (
	WorkspaceCreateInput struct {
		ID   string `json:"id"`
		Name string `json:"display_name"`
	}
)

// A snapshot contains configuration only. Never add records, credentials,
// assignments or approved origins here: it is also the release review boundary.
type SpaceConfiguration struct {
	Revision  int64                `json:"revision"`
	Entities  []ConfiguredEntity   `json:"entities"`
	Roles     []ConfiguredRole     `json:"roles"`
	Apps      []ConfiguredApp      `json:"apps"`
	Microapps []ConfiguredMicroapp `json:"microapps"`
}
type ConfiguredEntity struct {
	Revision   int64             `json:"revision"`
	ID         string            `json:"id"`
	State      string            `json:"state"`
	Definition entity.Definition `json:"definition"`
}
type ConfiguredRole struct {
	Revision int64  `json:"revision"`
	ID       string `json:"id"`
	App      string `json:"app_id,omitempty"`
	Code     string `json:"code"`
	Name     string `json:"display_name"`
	State    string `json:"state"`
}
type ConfiguredApp struct {
	Revision int64            `json:"revision"`
	ID       string           `json:"id"`
	Slug     string           `json:"slug"`
	Name     string           `json:"display_name"`
	State    string           `json:"state"`
	Access   entity.AppAccess `json:"access"`
}
type ConfiguredMicroapp struct {
	Revision int64  `json:"revision"`
	ID       string `json:"id"`
	App      string `json:"app_id"`
	Slug     string `json:"slug"`
	Name     string `json:"display_name"`
	Artifact string `json:"artifact_ref"`
	Order    int64  `json:"navigation_order"`
	State    string `json:"state"`
}

func (v *ConfiguredRole) ScanFields() []any {
	return []any{&v.Revision, &v.ID, &v.App, &v.Code, &v.Name, &v.State}
}

func (v *ConfiguredMicroapp) ScanFields() []any {
	return []any{&v.Revision, &v.ID, &v.App, &v.Slug, &v.Name, &v.Artifact, &v.Order, &v.State}
}

func (s *Service) snapshot(ctx context.Context, q registry.Queryer, space string) (SpaceConfiguration, error) {
	var definitionBytes int64
	if e := q.QueryRowContext(ctx, "SELECT COALESCE(SUM(OCTET_LENGTH(v.definition_json)),0) FROM entity_definitions e JOIN entity_versions v ON v.id=e.active_version_id WHERE e.space_id=?", space).Scan(&definitionBytes); e != nil {
		return SpaceConfiguration{}, e
	}
	if definitionBytes > registry.MaxSpaceSnapshotBytes {
		return SpaceConfiguration{}, registry.Conflict("Space configuration exceeds the workspace/release snapshot limit.")
	}
	out := SpaceConfiguration{Entities: []ConfiguredEntity{}, Roles: []ConfiguredRole{}, Apps: []ConfiguredApp{}, Microapps: []ConfiguredMicroapp{}}
	if e := q.QueryRowContext(ctx, "SELECT revision FROM spaces WHERE id=?", space).Scan(&out.Revision); e != nil {
		return out, e
	}
	var pending *string
	var definition registry.JSONValue
	var captured ConfiguredEntity
	r, e := q.QueryContext(ctx, "SELECT e.id,e.revision,e.state,e.pending_version_id,v.definition_json FROM entity_definitions e JOIN entity_versions v ON v.id=e.active_version_id WHERE e.space_id=? ORDER BY e.id", space)
	if e != nil {
		return out, e
	}
	defer r.Close()
	for r.Next() {
		if e = r.Scan(&captured.ID, &captured.Revision, &captured.State, &pending, &definition); e != nil {
			return out, e
		}
		if pending != nil {
			return out, registry.Conflict("Wait for entity publications to finish before capturing a workspace or release.")
		}
		if captured.Definition, e = entities.DefOf(definition.Value()); e != nil {
			return out, e
		}
		out.Entities = append(out.Entities, captured)
	}
	if e = r.Err(); e != nil {
		return out, e
	}
	if out.Roles, e = registry.ScanAll[ConfiguredRole](ctx, q, "SELECT revision,id,COALESCE(app_id,''),code,display_name,state FROM roles WHERE space_id=? AND domain='data' ORDER BY id", space); e != nil {
		return out, e
	}
	apps, e := registry.ScanAll[registry.AppRecord](ctx, q, "SELECT "+registry.AppRecordColumns+" FROM apps WHERE space_id=? ORDER BY id", space)
	if e != nil {
		return out, e
	}
	for _, v := range apps {
		a, e := registry.AccessOf(v.Access)
		if e != nil {
			return out, e
		}
		a.Origin = ""
		out.Apps = append(out.Apps, ConfiguredApp{v.Revision, v.ID, v.Slug, v.Name, v.State, a})
	}
	if out.Microapps, e = registry.ScanAll[ConfiguredMicroapp](ctx, q, "SELECT m.revision,m.id,m.app_id,m.slug,m.display_name,COALESCE(m.artifact_ref,''),m.navigation_order,m.state FROM microapp_installations m JOIN apps a ON a.id=m.app_id WHERE a.space_id=? ORDER BY m.id", space); e != nil {
		return out, e
	}
	encoded, _ := json.Marshal(out)
	if len(encoded) > registry.MaxSpaceSnapshotBytes {
		return out, registry.Conflict("Space configuration exceeds the workspace/release snapshot limit.")
	}
	return out, nil
}

// workspaceStorage selects a Workspace with its backing database resource
// and that resource's state; the query continues with a WHERE clause.
const workspaceStorage = "SELECT " + registry.WorkspaceColumns + ",sp.database_resource_id,d.state FROM development_workspaces w JOIN spaces sp ON sp.id=w.backing_space_id JOIN database_resources d ON d.id=sp.database_resource_id"

func scanWorkspaceStorage(ctx context.Context, q registry.Queryer, where string, args ...any) (registry.Workspace, error) {
	var v registry.Workspace
	return v, registry.Lookup(ctx, q, workspaceStorage+" "+where, args, append(v.ScanFields(), &v.DatabaseResource, &v.DatabaseState)...)
}

// WorkspaceItem is a workspace in a Space's collection.
type WorkspaceItem struct {
	// AgentClient names the agent client that owns an agent workspace.
	AgentClient    *string   `json:"agent_client_name"`
	BackingSpace   string    `json:"backing_space_id"`
	CreatedAt      time.Time `json:"created_at"`
	Name           string    `json:"display_name"`
	Error          *string   `json:"error_message"`
	ID             string    `json:"id"`
	OwnerGrant     *string   `json:"owner_agent_grant_id"`
	PackagePreview bool      `json:"package_preview,omitempty"`
	// PreparationStep is the WorkspaceSteps step a preparing workspace has
	// reached, or a failed one failed at.
	PreparationStep *string `json:"preparation_step"`
	ProductionSpace string  `json:"production_space_id"`
	Revision        int64   `json:"revision"`
	State           string  `json:"state"`
}

func (v *WorkspaceItem) ScanFields() []any {
	return []any{&v.AgentClient, &v.BackingSpace, &v.CreatedAt, &v.Name, &v.Error, &v.ID, &v.OwnerGrant, &v.PackagePreview, &v.PreparationStep, &v.ProductionSpace, &v.Revision, &v.State}
}

// workspaceIDs maps a workspace's development ids to production ids.
func workspaceIDs(ctx context.Context, q registry.Queryer, workspace string) (map[string]string, error) {
	mappings, e := registry.ScanAll[registry.WorkspaceMapping](ctx, q, "SELECT development_id,kind,production_id FROM workspace_items WHERE workspace_id=?", workspace)
	ids := map[string]string{}
	for _, m := range mappings {
		ids[m.Development] = m.Production
	}
	return ids, e
}

func (s *Service) workspaceRow(ctx context.Context, q registry.Queryer, p access.Principal, id string) (registry.Workspace, error) {
	v, e := scanWorkspaceStorage(ctx, q, "WHERE w.id=?", id)
	if e != nil {
		return v, e
	}
	return v, s.Access.WorkspaceOwner(ctx, q, p, v)
}

func (s *Service) Workspaces(ctx context.Context, actor op.Actor, space string, page op.PageRequest) (op.Page[WorkspaceItem], error) {
	p := actor.Principal
	var tenant string
	var parent *string
	if e := registry.Lookup(ctx, s.DB, "SELECT tenant_id,workspace_id FROM spaces WHERE id=?", []any{space}, &tenant, &parent); e != nil {
		return op.Page[WorkspaceItem]{}, e
	}
	if parent != nil {
		return op.Page[WorkspaceItem]{}, registry.Bad("Select the production Space.")
	}
	if e := s.Access.Authorize(ctx, s.DB, p, "workspace.manage", access.Resource{Tenant: tenant, Space: space}, true); e != nil {
		return op.Page[WorkspaceItem]{}, e
	}
	query := `SELECT g.client_name AS agent_client_name,w.backing_space_id,w.created_at,w.display_name,w.error_message,
		w.id,w.owner_agent_grant_id,d.package_preview,w.preparation_step,w.production_space_id,w.revision,w.state
		FROM development_workspaces w JOIN spaces sp ON sp.id=w.backing_space_id
		JOIN database_resources d ON d.id=sp.database_resource_id LEFT JOIN agent_grants g ON g.id=w.owner_agent_grant_id
		WHERE w.production_space_id=? AND w.owner_user_id=?`
	args := []any{space, p.ID}
	if p.Agent != nil {
		query += " AND w.owner_agent_grant_id=?"
		args = append(args, p.Agent.ID)
	}
	return op.PageScan[WorkspaceItem](ctx, s.DB, page, op.CollectionFor("workspaces"), query+" ORDER BY w.created_at DESC,w.id", args...)
}

func (s *Service) Workspace(ctx context.Context, actor op.Actor, workspace string) (registry.Workspace, error) {
	p := actor.Principal
	v, e := s.workspaceRow(ctx, s.DB, p, workspace)
	if e != nil {
		return v, e
	}
	mappings, e := registry.ScanAll[registry.WorkspaceMapping](ctx, s.DB, "SELECT development_id,kind,production_id FROM workspace_items WHERE workspace_id=? ORDER BY kind,development_id", v.ID)
	v.Mappings = &mappings
	return v, e
}

func (s *Service) CreateWorkspace(ctx context.Context, actor op.Actor, space string, in WorkspaceCreateInput) (registry.Workspace, error) {
	return s.StageWorkspace(ctx, actor, space, in, nil)
}

// StageWorkspace creates a workspace against the real production base and can
// add transactionally recorded, preplanned configuration to its private copy.
func (s *Service) StageWorkspace(ctx context.Context, actor op.Actor, space string, in WorkspaceCreateInput, stage func(context.Context, *sql.Tx, SpaceConfiguration) (SpaceConfiguration, error)) (registry.Workspace, error) {
	p := actor.Principal
	if in.ID == "" {
		in.ID = actor.OperationID()
	}
	if e := registry.ID(in.ID); e != nil {
		return registry.Workspace{}, e
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 160 {
		return registry.Workspace{}, registry.Bad("Enter a workspace name (maximum 160 bytes).")
	}
	res := access.Resource{Space: space}
	var parent *string
	if e := registry.Lookup(ctx, s.DB, "SELECT tenant_id,workspace_id FROM spaces WHERE id=? AND state='active'", []any{space}, &res.Tenant, &parent); e != nil {
		return registry.Workspace{}, e
	}
	if parent != nil {
		return registry.Workspace{}, registry.Bad("Workspaces cannot contain workspaces.")
	}
	e := s.Run.Mutate(ctx, actor, "workspace.manage", res, func(tx *sql.Tx) error {
		if e := CheckWorkspaceQuota(ctx, tx, res.Space, p.ID); e != nil {
			return e
		}
		if e := s.ensureSnapshotReady(ctx, tx, res.Space); e != nil {
			return e
		}
		base, e := s.snapshot(ctx, tx, res.Space)
		if e != nil {
			return e
		}
		b, _ := json.Marshal(base)
		database, policy, e := s.workspaceAllocation(ctx, tx, res, stage != nil)
		if e != nil {
			return e
		}
		backing := registry.NewID()
		database.ID = registry.NewID()
		if e = s.Storage.Allocate(ctx, tx, actor, res.Tenant, database, policy, &res); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO spaces(id,tenant_id,slug,display_name,database_resource_id,database_layout) VALUES(?,?,?,?,?,?)", backing, res.Tenant, "dev-"+strings.ReplaceAll(in.ID, "-", ""), in.Name, database.ID, database.Layout); e != nil {
			return e
		}
		var grant any
		if p.Agent != nil {
			grant = p.Agent.ID
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO development_workspaces(id,tenant_id,production_space_id,backing_space_id,owner_user_id,owner_agent_grant_id,display_name,base_hash,base_json,preparation_step) VALUES(?,?,?,?,?,?,?,?,?,?)", in.ID, res.Tenant, res.Space, backing, p.ID, grant, in.Name, entity.Hash(base), b, WorkspaceSteps[0]); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE spaces SET workspace_id=? WHERE id=?", in.ID, backing); e != nil {
			return e
		}
		if e = roles.AssignInitialManager(ctx, tx, actor, res.Tenant, backing, "", p.ID); e != nil {
			return e
		}
		if stage != nil {
			extra, err := stage(ctx, tx, base)
			if err != nil {
				return err
			}
			base.Entities = append(base.Entities, extra.Entities...)
			base.Apps = append(base.Apps, extra.Apps...)
			base.Microapps = append(base.Microapps, extra.Microapps...)
			base.Roles = append(base.Roles, extra.Roles...)
		}
		if e = s.installSnapshot(ctx, tx, base, res.Tenant, backing, in.ID); e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, actor, res.Tenant, "workspace.created", "workspace", in.ID, registry.Row{"production_space_id": res.Space, "backing_space_id": backing})
	})
	if e != nil {
		return registry.Workspace{}, e
	}
	return s.Workspace(ctx, actor, in.ID)
}

func (s *Service) workspaceAllocation(ctx context.Context, q registry.Queryer, res access.Resource, packagePreview bool) (databases.DatabaseInput, registry.Policy, error) {
	in := databases.DatabaseInput{Mode: "managed", PackagePreview: packagePreview}
	if e := registry.Lookup(ctx, q, "SELECT sp.database_layout,d.target_id FROM spaces sp JOIN database_resources d ON d.id=sp.database_resource_id WHERE sp.tenant_id=? AND sp.id=?", []any{res.Tenant, res.Space}, &in.Layout, &in.Target); e != nil {
		return in, registry.Policy{}, e
	}
	if s.Config.WorkspaceDatabaseTarget != "" {
		in.Target = s.Config.WorkspaceDatabaseTarget
	}
	policy, e := registry.PolicyFor(ctx, q, res.Tenant)
	return in, policy, e
}

func (s *Service) CheckPackagePreviewStorage(ctx context.Context, q registry.Queryer, res access.Resource) error {
	in, policy, e := s.workspaceAllocation(ctx, q, res, true)
	if e != nil {
		return e
	}
	return s.Storage.CheckAllocation(ctx, q, res.Tenant, in, policy, &res)
}

// entityVersionRef names an entity and one of its versions.
type entityVersionRef struct{ Entity, Version string }

func (v *entityVersionRef) ScanFields() []any { return []any{&v.Entity, &v.Version} }

func (s *Service) ChangeWorkspace(ctx context.Context, actor op.Actor, workspace string, retry bool, revision int64) (registry.Workspace, error) {
	p := actor.Principal
	v, e := s.workspaceRow(ctx, s.DB, p, workspace)
	if e != nil {
		return registry.Workspace{}, e
	}
	rev := revision
	if rev < 1 {
		return registry.Workspace{}, op.RevisionRequired()
	}
	e = s.Run.Mutate(ctx, actor, "workspace.manage", access.Resource{Tenant: v.TenantID, Space: v.ProductionSpace}, func(tx *sql.Tx) error {
		v, e = s.workspaceRow(ctx, tx, p, workspace)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(v.Revision, rev); e != nil {
			return e
		}
		state := registry.StateArchived
		if retry {
			if v.State != "failed" {
				return registry.Conflict("Only failed preparation can be retried.")
			}
			state = "preparing"
			failed, e := registry.ScanAll[entityVersionRef](ctx, tx, "SELECT e.id,v.id FROM entity_definitions e JOIN entity_versions v ON v.entity_id=e.id AND v.state='failed' WHERE e.space_id=? AND e.active_version_id IS NULL AND e.pending_version_id IS NULL", v.BackingSpace)
			if e != nil {
				return e
			}
			for _, def := range failed {
				if _, e = tx.ExecContext(ctx, "UPDATE entity_versions SET state='queued',attempt_id=NULL,error_message=NULL,finished_at=NULL WHERE id=?", def.Version); e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, "UPDATE entity_definitions SET pending_version_id=?,revision=revision+1 WHERE id=?", def.Version, def.Entity); e != nil {
					return e
				}
			}

			if _, e = tx.ExecContext(ctx, "UPDATE database_resources SET state='pending',revision=revision+1 WHERE id=? AND state='failed'", v.DatabaseResource); e != nil {
				return e
			}
		}
		// A retry starts again from the first preparation step.
		if _, e = tx.ExecContext(ctx, "UPDATE development_workspaces SET state=?,error_message=NULL,preparation_step=IF(?,?,preparation_step),revision=revision+1 WHERE id=?", state, retry, WorkspaceSteps[0], v.ID); e != nil {
			return e
		}
		if !retry {
			if _, e = tx.ExecContext(ctx, "UPDATE spaces SET state='archived',revision=revision+1 WHERE id=?", v.BackingSpace); e != nil {
				return e
			}
			if e = databases.SchedulePreviewCleanup(ctx, tx, v.ID); e != nil {
				return e
			}
		}
		return op.AuditAs(ctx, tx, actor, v.TenantID, "workspace."+state, "workspace", v.ID, registry.Row{})
	})
	if e != nil {
		return registry.Workspace{}, e
	}
	return s.Workspace(ctx, actor, workspace)
}

// CheckWorkspaceQuota is shared between read-only preflight and the locked
// workspace allocation. Only the latter can reserve a slot.
func CheckWorkspaceQuota(ctx context.Context, q registry.Queryer, space, user string) error {
	var owned, total int
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(owner_user_id=?),0) FROM development_workspaces WHERE production_space_id=? AND state<>'archived'", user, space).Scan(&total, &owned); e != nil {
		return e
	}
	if owned >= registry.MaxOwnedWorkspaces || total >= registry.MaxSpaceWorkspaces {
		return registry.Conflict("Archive an unused workspace before creating another.")
	}
	return nil
}
