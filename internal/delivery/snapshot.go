package delivery

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/data"
	"github.com/aantich/lastapp/internal/databases"
	"github.com/aantich/lastapp/internal/entities"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

func remapSnapshot(base SpaceConfiguration, ids map[string]string) SpaceConfiguration {
	// Only typed identity references are rewritten. Labels, formulas and arbitrary
	// literal JSON are data; a UUID-looking string is not automatically a reference.
	b, _ := json.Marshal(base)
	var out SpaceConfiguration
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	_ = d.Decode(&out)
	remap := func(v string) string {
		if n, ok := ids[v]; ok {
			return n
		}
		return v
	}
	permissions := map[string]string{}
	for i := range out.Entities {
		e := &out.Entities[i]
		old := e.ID
		e.ID = remap(e.ID)
		for _, p := range e.Definition.Permissions {
			permissions[entity.PermissionID(old, p.Key)] = entity.PermissionID(e.ID, p.Key)
		}
		e.Definition.RewriteReferences(remap, remap)
	}
	remapPermission := func(id string) string {
		if n := permissions[id]; n != "" {
			return n
		}
		return id
	}
	for i := range out.Roles {
		v := &out.Roles[i]
		v.ID = remap(v.ID)
		v.App = remap(v.App)
	}
	for i := range out.Apps {
		v := &out.Apps[i]
		v.ID = remap(v.ID)
		v.Access.Origin = ""
		v.Access.RewriteGateReferences(remapPermission, remap)
	}
	for i := range out.Microapps {
		v := &out.Microapps[i]
		v.ID = remap(v.ID)
		v.App = remap(v.App)
	}
	return out
}

func snapshotIDs(base SpaceConfiguration) map[string]string {
	ids := map[string]string{}
	for _, v := range base.Entities {
		ids[v.ID] = "entity"
	}
	for _, v := range base.Roles {
		ids[v.ID] = "role"
	}
	for _, v := range base.Apps {
		ids[v.ID] = "app"
	}
	for _, v := range base.Microapps {
		ids[v.ID] = "microapp"
	}
	return ids
}

func (s *Service) installSnapshot(ctx context.Context, tx *sql.Tx, base SpaceConfiguration, tenant, space, workspace string) error {
	if len(base.Apps) > registry.MaxWorkspaceApps {
		return registry.Conflict("Space exceeds the workspace App limit.")
	}
	if e := registry.MetadataQuota(ctx, tx, tenant, "roles", len(base.Roles)); e != nil {
		return e
	}
	if e := registry.MetadataQuota(ctx, tx, tenant, "microapp_installations", len(base.Microapps)); e != nil {
		return e
	}
	ids := map[string]string{}
	for old, kind := range snapshotIDs(base) {
		ids[old] = registry.NewID()
		if _, e := tx.ExecContext(ctx, "INSERT INTO workspace_items(workspace_id,kind,development_id,production_id) VALUES(?,?,?,?)", workspace, kind, ids[old], old); e != nil {
			return e
		}
	}
	copy := remapSnapshot(base, ids)
	for _, v := range copy.Apps {
		b, _ := json.Marshal(v.Access)
		if _, e := tx.ExecContext(ctx, "INSERT INTO apps(id,tenant_id,space_id,slug,display_name,state,compute_mode,data_access_json) VALUES(?,?,?,?,?,?,'shared',?)", v.ID, tenant, space, v.Slug, v.Name, v.State, b); e != nil {
			return e
		}
	}
	for _, v := range copy.Roles {
		kind, owner := "space", "s:"+space
		if v.App != "" {
			kind, owner = "app", "a:"+v.App
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO roles(id,tenant_id,space_id,app_id,owner_key,scope_kind,domain,code,display_name,state) VALUES(?,?,?,?,?,?,'data',?,?,?)", v.ID, tenant, space, registry.Nullable(v.App), owner, kind, v.Code, v.Name, v.State); e != nil {
			return e
		}
	}
	for _, v := range copy.Entities {
		b, _ := json.Marshal(v.Definition)
		if _, e := tx.ExecContext(ctx, "INSERT INTO entity_definitions(id,tenant_id,space_id,entity_key,label,state,draft_json) VALUES(?,?,?,?,?,?,?)", v.ID, tenant, space, v.Definition.Key, v.Definition.Label, v.State, b); e != nil {
			return e
		}
	}
	for _, v := range copy.Microapps {
		if _, e := tx.ExecContext(ctx, "INSERT INTO microapp_installations(id,tenant_id,app_id,slug,display_name,artifact_ref,navigation_order,state) VALUES(?,?,?,?,?,?,?,?)", v.ID, tenant, v.App, v.Slug, v.Name, registry.Nullable(v.Artifact), v.Order, v.State); e != nil {
			return e
		}
	}
	return nil
}

// The single publication capture routine is shared by the authoring endpoint and
// bounded snapshot jobs. Candidate references are accepted only for a captured
// batch whose tables are prepared before any version can activate.
func (s *Service) QueueDefinition(ctx context.Context, tx *sql.Tx, p access.Principal, v entities.EntityDefinition, version string, candidates map[string]bool) error {
	d, e := v.DraftDefinition()
	if e != nil {
		return e
	}
	if ds := d.Validate(); len(ds) > 0 {
		return registry.Bad(ds[0].Path + ": " + ds[0].Message)
	}
	if e = entities.ValidateEntityRefsCandidates(ctx, tx, d, v.TenantID, v.SpaceID, v.ID, candidates); e != nil {
		return e
	}
	if v.PendingVersion != nil {
		return registry.Conflict("A publication is already pending.")
	}
	b, _ := json.Marshal(d)
	plan := data.Plan(v.ID, data.TableMarker(s.RegistryName, v.ID), d)
	pb, _ := json.Marshal(plan)
	// The entity row lock serialises every capture of this entity, and the locking
	// read sees the latest committed number, so concurrent publications get
	// distinct increasing numbers.
	var locked string
	if e = tx.QueryRowContext(ctx, "SELECT id FROM entity_definitions WHERE id=? FOR UPDATE", v.ID).Scan(&locked); e != nil {
		return e
	}
	var number int64
	if e = tx.QueryRowContext(ctx, entities.NextVersionNumber+" FOR UPDATE", v.ID).Scan(&number); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO entity_versions(id,tenant_id,space_id,entity_id,version_number,base_version_id,definition_json,definition_hash,created_by,storage_plan_json,storage_plan_hash,agent_grant_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", version, v.TenantID, v.SpaceID, v.ID, number, v.ActiveVersion, b, entity.Hash(d), p.ID, pb, entity.Hash(plan), agentID(p)); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE entity_definitions SET pending_version_id=?,revision=revision+1 WHERE id=?", version, v.ID)
	return e
}

func agentID(p access.Principal) any {
	if p.Agent != nil {
		return p.Agent.ID
	}
	return nil
}

// Empty tables are prepared without foreign keys, then the ordinary publisher
// verifies/adds those keys before activation. This also supports cyclic references.
func (s *Service) prepareSnapshotTables(ctx context.Context, resource string, entities []entities.EntityDefinition) error {
	db, e := s.Publisher.BusinessDB(ctx, resource, true)
	if e != nil {
		return e
	}
	defer db.Close()
	for _, v := range entities {
		if v.ActiveVersion != nil {
			continue
		}
		d, e := v.DraftDefinition()
		if e != nil {
			return e
		}
		if ds := d.Validate(); len(ds) > 0 {
			return registry.Bad(ds[0].Message)
		}
		marker := data.TableMarker(s.RegistryName, v.ID)
		var comment string
		e = db.QueryRowContext(ctx, "SELECT TABLE_COMMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", strings.Trim(data.Table(v.ID), "`")).Scan(&comment)
		if e == nil {
			if comment != marker {
				return databases.OwnershipConflict()
			}
			continue
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if _, e = db.ExecContext(ctx, data.SkeletonSQL(v.ID, marker, d)); e != nil {
			return e
		}
	}
	return nil
}

// WorkspaceJob advances one preparing workspace of the first Tenant after the
// given one (Tenant ID order) that has any. It tries that Tenant's workspaces
// oldest first and stops at the first that progresses; workspaces are
// independent (each has its own backing Space and storage), so one waiting
// for its storage or publications never holds back another. It returns the
// Tenant, or "" when no later Tenant has a preparing workspace, and whether a
// workspace progressed. An error (including a deadlock or lock wait timeout,
// which never fails a workspace) does not stop the other workspaces of the
// pass; the first one is returned.
func (s *Service) WorkspaceJob(ctx context.Context, q registry.Queryer, after string) (tenant string, progressed bool, err error) {
	if e := registry.Lookup(ctx, q, "SELECT tenant_id FROM development_workspaces WHERE state='preparing' AND tenant_id>? ORDER BY tenant_id LIMIT 1", []any{after}, &tenant); e != nil {
		if ae, ok := e.(*registry.APIError); ok && ae.Status == 404 {
			return "", false, nil
		}
		return "", false, e
	}
	rows, e := q.QueryContext(ctx, "SELECT id FROM development_workspaces WHERE tenant_id=? AND state='preparing' ORDER BY created_at,id", tenant)
	if e != nil {
		return tenant, false, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return tenant, false, e
		}
		ids = append(ids, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return tenant, false, e
	}
	for _, id := range ids {
		v, e := scanWorkspaceStorage(ctx, q, "WHERE w.id=? AND w.state='preparing'", id)
		if ae, ok := e.(*registry.APIError); ok && ae.Status == 404 {
			continue // Finished or archived meanwhile.
		}
		if e == nil {
			progressed, e = s.prepareWorkspace(ctx, q, v)
		}
		if e != nil {
			if err == nil {
				err = e
			}
			continue
		}
		if progressed {
			return tenant, true, nil
		}
	}
	return tenant, false, err
}

// prepareWorkspace advances one preparing workspace. It reports false while
// the workspace waits for its storage or its entity publications.
func (s *Service) prepareWorkspace(ctx context.Context, q registry.Queryer, v registry.Workspace) (bool, error) {
	var e error
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, access.WorkspaceOperationKey{}, v.ID)
	p := access.Principal{ID: v.Owner}
	if access.Deref(v.OwnerGrant) != "" {
		p.Agent, e = s.Access.ReadAgentGrant(ctx, q, access.Deref(v.OwnerGrant))
		if e != nil {
			return false, e
		}
	}
	waiting := false
	run := func() error {
		if p.Agent != nil {
			if e = access.RequireAgent(p.Agent, "workspaces.write"); e != nil {
				return e
			}
		}
		if e = s.Access.WorkspaceOwner(ctx, q, p, v); e != nil {
			return e
		}
		if access.Deref(v.DatabaseState) == "pending" || access.Deref(v.DatabaseState) == "provisioning" {
			waiting = true
			return nil
		}
		if access.Deref(v.DatabaseState) != "ready" {
			return registry.Conflict("Workspace storage failed. Inspect database setup and retry workspace preparation.")
		}
		defs, e := registry.ScanAll[entities.EntityDefinition](ctx, q, "SELECT "+entities.EntityColumns+" FROM entity_definitions WHERE space_id=? ORDER BY id", v.BackingSpace)
		if e != nil {
			return e
		}
		if e = s.workspaceStep(ctx, q, v, WorkspaceSteps[1]); e != nil {
			return e
		}
		if e = s.prepareSnapshotTables(ctx, access.Deref(v.DatabaseResource), defs); e != nil {
			return e
		}
		candidates := map[string]bool{}
		for _, d := range defs {
			candidates[d.ID] = true
		}
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = registry.LockTenant(ctx, tx, v.TenantID); e != nil {
			return e
		}
		if e = s.Access.WorkspaceOwner(ctx, tx, p, v); e != nil {
			return e
		}
		ready, queued := true, false
		for _, d := range defs {
			if d.ActiveVersion != nil {
				continue
			}
			ready = false
			if d.PendingVersion != nil {
				continue
			}
			// A previous failure is surfaced rather than silently retried forever.
			var failures int
			if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM entity_versions WHERE entity_id=? AND state='failed'", d.ID).Scan(&failures); e != nil {
				return e
			}
			if failures > 0 {
				return registry.Conflict("Workspace entity preparation failed. Review publications before retrying.")
			}
			// The workspace job owns baseline publication authority; ordinary later
			// agent publications keep their own grants and permission checks.
			if e = s.QueueDefinition(ctx, tx, access.Principal{ID: p.ID}, d, registry.NewID(), candidates); e != nil {
				return e
			}
			queued = true
		}
		// Unpublished entities that are all already queued: wait for them.
		waiting = !ready && !queued
		if !ready {
			if e = s.workspaceStep(ctx, tx, v, WorkspaceSteps[2]); e != nil {
				return e
			}
		}
		if ready {
			apps, err := registry.ScanAll[registry.AppRecord](ctx, tx, "SELECT "+registry.AppRecordColumns+" FROM apps WHERE space_id=?", v.BackingSpace)
			if err != nil {
				return err
			}
			for _, a := range apps {
				access, err := registry.AccessOf(a.Access)
				if err != nil {
					return err
				}
				if err = entities.ProjectAppPermissions(ctx, tx, a.ID, access); err != nil {
					return err
				}
			}
			if _, e = tx.ExecContext(ctx, "UPDATE development_workspaces SET state='ready',preparation_step=NULL,revision=revision+1 WHERE id=? AND state='preparing'", v.ID); e != nil {
				return e
			}
		}
		return tx.Commit()
	}
	if e = run(); e == nil {
		return !waiting, nil
	}
	if op.Transient(e) {
		return false, e
	}
	message := "Workspace preparation failed. Check storage and publications, then retry."
	if ae, ok := e.(*registry.APIError); ok {
		message = ae.Message
	} else {
		s.Warn(ctx, "workspace preparation failed", e)
	}
	_, e = q.ExecContext(parent, "UPDATE development_workspaces SET state='failed',error_message=?,revision=revision+1 WHERE id=? AND state='preparing'", message, v.ID)
	return true, e
}

// workspaceStep records that a preparing workspace has reached step (one of
// WorkspaceSteps). It never moves a workspace back, so a pass that repeats an
// earlier step (waiting for publications) leaves the later one in place.
func (s *Service) workspaceStep(ctx context.Context, q registry.Queryer, v registry.Workspace, step string) error {
	_, e := q.ExecContext(ctx, "UPDATE development_workspaces SET preparation_step=?,revision=revision+1 WHERE id=? AND state='preparing' AND FIND_IN_SET(COALESCE(preparation_step,''),?)<FIND_IN_SET(?,?)", step, v.ID, strings.Join(WorkspaceSteps, ","), step, strings.Join(WorkspaceSteps, ","))
	return e
}

func snapshotContractHash(v SpaceConfiguration) string {
	b, _ := json.Marshal(v)
	var copy SpaceConfiguration
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	_ = d.Decode(&copy)
	copy.Revision = 0
	for i := range copy.Entities {
		copy.Entities[i].Revision = 0
	}
	for i := range copy.Roles {
		copy.Roles[i].Revision = 0
	}
	for i := range copy.Apps {
		copy.Apps[i].Revision = 0
	}
	for i := range copy.Microapps {
		copy.Microapps[i].Revision = 0
	}
	return entity.Hash(copy)
}

// publishedDraft is an entity draft with its pending publication and the
// hash of its active definition ("" before the first publication).
type publishedDraft struct {
	Draft      registry.JSONValue
	Pending    *string
	ActiveHash string
}

func (v *publishedDraft) ScanFields() []any { return []any{&v.Draft, &v.Pending, &v.ActiveHash} }

func (s *Service) ensureSnapshotReady(ctx context.Context, q registry.Queryer, space string) error {
	drafts, e := registry.ScanAll[publishedDraft](ctx, q, "SELECT e.draft_json,e.pending_version_id,COALESCE(ev.definition_hash,'') FROM entity_definitions e LEFT JOIN entity_versions ev ON ev.id=e.active_version_id WHERE e.space_id=? AND e.state='active'", space)
	if e != nil {
		return e
	}
	for _, draft := range drafts {
		d, e := entities.DefOf(draft.Draft.Value())
		if e != nil {
			return e
		}
		if draft.Pending != nil || entity.Hash(d) != draft.ActiveHash {
			return registry.Conflict("Publish every active entity draft before capturing a workspace or release.")
		}
	}
	return nil
}
