package delivery

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/entities"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/inventory"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

// ProductionConfiguration reads the bounded Space snapshot used by both
// workspace capture and installation planning.
func (s *Service) ProductionConfiguration(ctx context.Context, q registry.Queryer, space string) (SpaceConfiguration, error) {
	if e := s.ensureSnapshotReady(ctx, q, space); e != nil {
		return SpaceConfiguration{}, e
	}
	return s.snapshot(ctx, q, space)
}

func (s *Service) ConfigurationQuota(ctx context.Context, q registry.Queryer, tenant string, c SpaceConfiguration) error {
	if len(c.Apps) > 0 {
		if e := s.snapshotAppQuota(ctx, q, tenant, c); e != nil {
			return e
		}
	}
	if len(c.Roles) > 0 {
		if e := registry.MetadataQuota(ctx, q, tenant, "roles", len(c.Roles)); e != nil {
			return e
		}
	}
	if len(c.Microapps) > 0 {
		if e := registry.MetadataQuota(ctx, q, tenant, "microapp_installations", len(c.Microapps)); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) PackagePreview(ctx context.Context, tx *sql.Tx, actor op.Actor, workspace, productionApp, name string, build artifact.Build, manifest ReleaseManifest, source string) (string, string, error) {
	var backing, developmentApp, baseHash, tenant string
	e := registry.Lookup(ctx, tx, `SELECT ws.tenant_id,ws.backing_space_id,ws.base_hash,wi.development_id
		FROM development_workspaces ws JOIN workspace_items wi ON wi.workspace_id=ws.id AND wi.kind='app' AND wi.production_id=?
		WHERE ws.id=? AND ws.state='ready'`, []any{productionApp, workspace}, &tenant, &backing, &baseHash, &developmentApp)
	if e != nil {
		return "", "", e
	}
	if e = s.Access.WorkspaceGuard(ctx, tx, actor.Principal, backing); e != nil {
		return "", "", e
	}
	if e = s.Run.AuthorizeTx(ctx, tx, actor, "release.manage", access.Resource{Tenant: tenant, App: developmentApp}); e != nil {
		return "", "", e
	}
	a, e := inventory.AppRow(ctx, tx, developmentApp)
	if e != nil {
		return "", "", e
	}
	current, e := s.ProductionConfiguration(ctx, tx, backing)
	if e != nil {
		return "", "", e
	}
	ids, e := workspaceIDs(ctx, tx, workspace)
	if e != nil {
		return "", "", e
	}
	mapped := remapSnapshot(current, ids)
	buildID, release := registry.NewID(), registry.NewID()
	if e = insertBuild(ctx, tx, buildID, tenant, developmentApp, actor.Principal.ID, build, source); e != nil {
		return "", "", e
	}
	if _, e = insertRelease(ctx, tx, release, a, workspace, baseHash, actor.Principal.ID, ReleaseCreateInput{Name: name, Build: buildID, Manifest: manifest}, mapped); e != nil {
		return "", "", e
	}
	origin := s.HostedOrigin(developmentApp)
	if origin == "" {
		return "", "", registry.Conflict("Configure the App hosting origin first.")
	}
	if e = selectPreview(ctx, tx, a, release, origin); e != nil {
		return "", "", e
	}
	return release, origin + "/", nil
}

// QueuePackage uses the same deployment queue and publisher as ordinary App
// releases. It receives only the explicit writes approved by the caller.
func (s *Service) QueuePackage(ctx context.Context, tx *sql.Tx, actor op.Actor, res access.Resource, kind, baseHash, release string, c SpaceConfiguration, audience []string) (string, error) {
	if e := s.deploymentAuthority(ctx, tx, actor.Principal, res); e != nil {
		return "", e
	}
	if e := entities.CheckSpaceCapacity(ctx, tx, res.Space, len(c.Entities)); e != nil {
		return "", e
	}
	if e := s.ConfigurationQuota(ctx, tx, res.Tenant, c); e != nil {
		return "", e
	}
	operation := actor.OperationID()
	var appID any
	if len(c.Apps) > 0 {
		app := c.Apps[0]
		appID = app.ID
		// Reserve the foreign-key target without making it launchable.
		if _, e := tx.ExecContext(ctx, "INSERT INTO apps(id,tenant_id,space_id,slug,display_name,state,compute_mode) VALUES(?,?,?,?,?,'archived','shared')", app.ID, res.Tenant, res.Space, app.Slug, app.Name); e != nil {
			return "", e
		}
	}
	plan := deploymentPlan{Snapshot: c, BaseHash: baseHash, Versions: map[string]string{}, Audience: audience}
	for _, d := range c.Entities {
		plan.Versions[d.ID] = registry.NewID()
	}
	b, _ := json.Marshal(plan)
	var grantRev any
	if actor.Principal.Agent != nil {
		grantRev = actor.Principal.Agent.Revision
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO release_deployments(id,tenant_id,space_id,release_id,target_app_id,created_by,agent_grant_id,agent_grant_revision,plan_json,kind)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, operation, res.Tenant, res.Space, registry.Nullable(release), appID, actor.Principal.ID, agentID(actor.Principal), grantRev, b, kind)
	if e != nil {
		return "", e
	}
	return operation, op.AuditAs(ctx, tx, actor, res.Tenant, "package.activation_queued", "deployment", operation, registry.Row{"kind": kind, "base_hash": baseHash, "configuration_hash": entity.Hash(c)})
}

// CheckPackageWorkspace verifies that the reviewed additions still match the
// private workspace and that its other resources still match the frozen base.
func (s *Service) CheckPackageWorkspace(ctx context.Context, q registry.Queryer, actor op.Actor, workspace, baseHash string, added SpaceConfiguration) error {
	var backing, owner string
	var grant *string
	var raw []byte
	e := registry.Lookup(ctx, q, "SELECT backing_space_id,owner_user_id,owner_agent_grant_id,base_json FROM development_workspaces WHERE id=? AND state='ready' AND base_hash=?", []any{workspace, baseHash}, &backing, &owner, &grant, &raw)
	if e != nil {
		return e
	}
	if owner != actor.Principal.ID || access.Deref(grant) != access.Deref(agentGrantPointer(actor.Principal)) {
		return registry.Missing()
	}
	var expected SpaceConfiguration
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e = d.Decode(&expected); e != nil {
		return e
	}
	expected.Entities = append(expected.Entities, added.Entities...)
	expected.Roles = append(expected.Roles, added.Roles...)
	expected.Apps = append(expected.Apps, added.Apps...)
	expected.Microapps = append(expected.Microapps, added.Microapps...)
	current, e := s.ProductionConfiguration(ctx, q, backing)
	if e != nil {
		return e
	}
	ids, e := workspaceIDs(ctx, q, workspace)
	if e != nil {
		return e
	}
	if normalizedConfigurationHash(expected) != normalizedConfigurationHash(remapSnapshot(current, ids)) {
		return registry.Conflict("The staged workspace changed after review.")
	}
	return nil
}

func agentGrantPointer(p access.Principal) *string {
	if p.Agent != nil {
		return &p.Agent.ID
	}
	return nil
}

func normalizedConfigurationHash(v SpaceConfiguration) string {
	v.Entities = append([]ConfiguredEntity{}, v.Entities...)
	v.Apps = append([]ConfiguredApp{}, v.Apps...)
	v.Roles = append([]ConfiguredRole{}, v.Roles...)
	v.Microapps = append([]ConfiguredMicroapp{}, v.Microapps...)
	slices.SortFunc(v.Entities, func(a, b ConfiguredEntity) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(v.Apps, func(a, b ConfiguredApp) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(v.Roles, func(a, b ConfiguredRole) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(v.Microapps, func(a, b ConfiguredMicroapp) int { return strings.Compare(a.ID, b.ID) })
	return snapshotContractHash(v)
}
