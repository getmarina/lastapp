package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/inventory"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

type ReleaseManifest struct {
	Microapps    map[string]string `json:"microapps"`
	Dependencies map[string]string `json:"dependencies"`
}
type ReleaseCreateInput struct {
	Name     string          `json:"display_name"`
	Build    string          `json:"build_id"`
	Manifest ReleaseManifest `json:"manifest"`
}

// One public status projection for discovery, polling and MCP. Never expose the
// persisted plan or delegation details through a status response.
// Deployment is a production deployment's status and the URL it serves.
type Deployment struct {
	// Attempts counts the deployment's runs: 1, plus one per retry.
	Attempts   int64     `json:"attempts"`
	CreatedAt  time.Time `json:"created_at"`
	DeployedBy string    `json:"deployed_by"`
	Error      *string   `json:"error_message"`
	// FailedState is the step a failed deployment had reached.
	FailedState *string    `json:"failed_state"`
	FinishedAt  *time.Time `json:"finished_at"`
	ID          string     `json:"id"`
	ReleaseID   string     `json:"release_id"`
	Release     string     `json:"release_name"`
	// RetriedBy names the person who last retried it; null before a retry.
	RetriedBy *string `json:"retried_by"`
	Revision  int64   `json:"revision"`
	SpaceID   string  `json:"space_id"`
	State     string  `json:"state"`
	TargetApp string  `json:"target_app_id"`
	TenantID  string  `json:"tenant_id"`
	URL       string  `json:"url"`
}

// The status names who started the deployment (the person; an agent acts for
// one) and the release's name, so a status needs no further reads to display.
const (
	deploymentStatusColumns = "d.attempts,d.created_at,u.display_name AS deployed_by,d.error_message,d.failed_state,d.finished_at,d.id,COALESCE(d.release_id,'') AS release_id,COALESCE(rel.display_name,'') AS release_name,r.display_name AS retried_by,d.revision,d.space_id,d.state,COALESCE(d.target_app_id,'') AS target_app_id,d.tenant_id"
	deploymentStatusFrom    = " FROM release_deployments d LEFT JOIN app_releases rel ON rel.id=d.release_id JOIN users u ON u.id=d.created_by LEFT JOIN users r ON r.id=d.retried_by"
)

func (v *Deployment) ScanFields() []any {
	return []any{&v.Attempts, &v.CreatedAt, &v.DeployedBy, &v.Error, &v.FailedState, &v.FinishedAt, &v.ID, &v.ReleaseID, &v.Release, &v.RetriedBy, &v.Revision, &v.SpaceID, &v.State, &v.TargetApp, &v.TenantID}
}

// Release is a captured App release with its workspace ownership and
// production target. The release view adds its preview and production URLs.
type Release struct {
	AppID           string             `json:"app_id"`
	BackingSpace    string             `json:"backing_space_id"`
	BaseHash        string             `json:"base_hash"`
	BuildID         string             `json:"build_id"`
	CreatedAt       time.Time          `json:"created_at"`
	CreatedBy       string             `json:"created_by"`
	Name            string             `json:"display_name"`
	ID              string             `json:"id"`
	Manifest        registry.JSONValue `json:"manifest_json"`
	OwnerGrant      *string            `json:"owner_agent_grant_id"`
	Owner           string             `json:"owner_user_id"`
	PreviewURL      string             `json:"preview_url,omitempty"`
	ProductionApp   string             `json:"production_app_id"`
	ProductionSpace string             `json:"production_space_id"`
	ProductionURL   string             `json:"production_url,omitempty"`
	SnapshotHash    string             `json:"snapshot_hash"`
	Snapshot        registry.JSONValue `json:"snapshot_json"`
	TenantID        string             `json:"tenant_id"`
	WorkspaceID     string             `json:"workspace_id"`
}

func (v *Release) ScanFields() []any {
	return []any{&v.AppID, &v.BackingSpace, &v.BaseHash, &v.BuildID, &v.CreatedAt, &v.CreatedBy, &v.Name, &v.ID, &v.Manifest, &v.OwnerGrant, &v.Owner, &v.ProductionApp, &v.ProductionSpace, &v.SnapshotHash, &v.Snapshot, &v.TenantID, &v.WorkspaceID}
}

// ReleaseItem is a release in an App's collection. It names who captured it:
// the person and, when an agent owns the release's workspace, that agent
// client acting for them.
type ReleaseItem struct {
	AgentClient  *string   `json:"agent_client_name"`
	BuildID      string    `json:"build_id"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by_name"`
	Name         string    `json:"display_name"`
	ID           string    `json:"id"`
	SnapshotHash string    `json:"snapshot_hash"`
}

func (v *ReleaseItem) ScanFields() []any {
	return []any{&v.AgentClient, &v.BuildID, &v.CreatedAt, &v.CreatedBy, &v.Name, &v.ID, &v.SnapshotHash}
}

type deploymentPlan struct {
	Audience []string           `json:"launch_roles,omitempty"`
	Snapshot SpaceConfiguration `json:"snapshot"`
	BaseHash string             `json:"base_hash"`
	Versions map[string]string  `json:"versions"`
}

var (
	pinnedNameRE    = regexp.MustCompile(`^[a-zA-Z0-9@][a-zA-Z0-9._/@-]{0,159}$`)
	pinnedVersionRE = regexp.MustCompile(`^(?:[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?|sha256:[a-f0-9]{64})$`)
)

func (s *Service) CreateRelease(ctx context.Context, actor op.Actor, app string, in ReleaseCreateInput, revision int64) (Release, error) {
	p := actor.Principal
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 160 || registry.ID(in.Build) != nil {
		return Release{}, registry.Bad("Provide a release name and uploaded build ID.")
	}
	rev := revision
	if rev < 1 {
		return Release{}, op.RevisionRequired()
	}
	a, ws, e := s.ownedDevelopmentApp(ctx, s.DB, p, app)
	if e != nil {
		return Release{}, e
	}
	record := actor.OperationID()
	e = s.Run.Mutate(ctx, actor, "release.manage", access.Resource{Tenant: a.TenantID, App: a.ID}, func(tx *sql.Tx) error {
		a, e = inventory.AppRow(ctx, tx, a.ID)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(a.Revision, rev); e != nil {
			return e
		}
		var build string
		if e = registry.Lookup(ctx, tx, "SELECT id FROM app_builds WHERE id=? AND app_id=?", []any{in.Build, a.ID}, &build); e != nil {
			return e
		}
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_releases WHERE app_id=?", a.ID).Scan(&count); e != nil {
			return e
		}
		if count >= registry.MaxAppReleases {
			return registry.Conflict("Release limit reached. Start a fresh workspace.")
		}
		if e = s.ensureSnapshotReady(ctx, tx, a.SpaceID); e != nil {
			return e
		}
		base, e := s.snapshot(ctx, tx, a.SpaceID)
		if e != nil {
			return e
		}
		if len(in.Manifest.Microapps) > registry.MaxReleaseMicroapps || len(in.Manifest.Dependencies) > registry.MaxReleaseDependencies {
			return registry.Bad("Release manifest is too large.")
		}
		needed := map[string]bool{}
		for _, v := range base.Microapps {
			if v.App == a.ID && v.State == registry.StateActive {
				needed[v.Slug] = true
				if !pinnedVersionRE.MatchString(in.Manifest.Microapps[v.Slug]) {
					return registry.Bad("Pin a version for each active micro-app in this App.")
				}
			}
		}
		for key := range in.Manifest.Microapps {
			if !needed[key] {
				return registry.Bad("Manifest names a micro-app outside this App.")
			}
		}
		for name, version := range in.Manifest.Dependencies {
			if len(name) > 160 || !pinnedNameRE.MatchString(name) || !pinnedVersionRE.MatchString(version) {
				return registry.Bad("Dependencies require bounded package names and exact versions (no ranges or latest).")
			}
		}
		// Complete mappings are allocated once and survive later release captures.
		ids, e := workspaceIDs(ctx, tx, ws.ID)
		if e != nil {
			return e
		}
		for key, kind := range snapshotIDs(base) {
			if ids[key] == "" {
				ids[key] = registry.NewID()
				if _, e = tx.ExecContext(ctx, "INSERT INTO workspace_items(workspace_id,kind,development_id,production_id) VALUES(?,?,?,?)", ws.ID, kind, key, ids[key]); e != nil {
					return e
				}
			}
		}
		production := remapSnapshot(base, ids)
		hash, err := insertRelease(ctx, tx, record, a, ws.ID, ws.BaseHash, p.ID, in, production)
		if err != nil {
			return err
		}
		return op.AuditAs(ctx, tx, actor, a.TenantID, "release.created", "release", record, registry.Row{"build_id": in.Build, "snapshot_hash": hash})
	})
	if e != nil {
		return Release{}, e
	}
	return s.Release(ctx, actor, record)
}

func (s *Service) Releases(ctx context.Context, actor op.Actor, app string, page op.PageRequest) (op.Page[ReleaseItem], error) {
	p := actor.Principal
	a, e := inventory.AppRow(ctx, s.DB, app)
	if e != nil {
		return op.Page[ReleaseItem]{}, e
	}
	if e = s.Access.Authorize(ctx, s.DB, p, "release.manage", access.Resource{Tenant: a.TenantID, App: a.ID}, false); e != nil {
		return op.Page[ReleaseItem]{}, e
	}
	vs, e := op.PageScan[ReleaseItem](ctx, s.DB, page, op.CollectionFor("releases"), "SELECT g.client_name AS agent_client_name,rel.build_id,rel.created_at,u.display_name AS created_by_name,rel.display_name,rel.id,rel.snapshot_hash FROM app_releases rel JOIN users u ON u.id=rel.created_by JOIN development_workspaces ws ON ws.id=rel.workspace_id LEFT JOIN agent_grants g ON g.id=ws.owner_agent_grant_id WHERE rel.app_id=? OR EXISTS(SELECT 1 FROM release_deployments d WHERE d.release_id=rel.id AND d.target_app_id=? AND d.state='active') ORDER BY rel.created_at DESC,rel.id", a.ID, a.ID)
	return vs, e
}

func (s *Service) releaseRow(ctx context.Context, q registry.Queryer, p access.Principal, id string) (Release, error) {
	v, e := registry.ScanOne[Release](ctx, q, "SELECT rel.app_id,ws.backing_space_id,rel.base_hash,rel.build_id,rel.created_at,rel.created_by,rel.display_name,rel.id,rel.manifest_json,ws.owner_agent_grant_id,ws.owner_user_id,wi.production_id,ws.production_space_id,rel.snapshot_hash,rel.snapshot_json,rel.tenant_id,rel.workspace_id FROM app_releases rel JOIN development_workspaces ws ON ws.id=rel.workspace_id JOIN workspace_items wi ON wi.workspace_id=ws.id AND wi.kind='app' AND wi.development_id=rel.app_id WHERE rel.id=?", id)
	if e != nil {
		return v, e
	}
	if e = s.Access.Authorize(ctx, q, p, "release.manage", access.Resource{Tenant: v.TenantID, Space: v.ProductionSpace}, false); e != nil {
		return v, e
	}
	// Undeployed snapshots belong to the private workspace. Successful releases
	// remain inspectable as production history after that workspace is archived.
	var deployed int
	if e = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM release_deployments WHERE release_id=? AND tenant_id=? AND space_id=? AND target_app_id=? AND state='active'", id, v.TenantID, v.ProductionSpace, v.ProductionApp).Scan(&deployed); e != nil {
		return v, e
	}
	if deployed == 0 {
		if e = s.Access.WorkspaceGuard(ctx, q, p, v.BackingSpace); e != nil {
			return v, e
		}
	}
	return v, nil
}

func (s *Service) Release(ctx context.Context, actor op.Actor, release string) (Release, error) {
	p := actor.Principal
	v, e := s.releaseRow(ctx, s.DB, p, release)
	if e != nil {
		return v, e
	}
	v.PreviewURL = s.HostedOrigin(v.AppID) + "/"
	v.ProductionURL = s.HostedOrigin(v.ProductionApp) + "/"
	return v, nil
}

func (s *Service) PreviewRelease(ctx context.Context, actor op.Actor, release string, revision int64) (registry.Row, error) {
	p := actor.Principal
	rel, e := s.releaseRow(ctx, s.DB, p, release)
	if e != nil {
		return nil, e
	}
	a, _, e := s.ownedDevelopmentApp(ctx, s.DB, p, rel.AppID)
	if e != nil {
		return nil, e
	}
	rev := revision
	if rev < 1 {
		return nil, op.RevisionRequired()
	}
	origin := s.HostedOrigin(a.ID)
	if origin == "" {
		return nil, registry.Conflict("Configure the App hosting origin first.")
	}
	e = s.Run.Mutate(ctx, actor, "release.manage", access.Resource{Tenant: a.TenantID, App: a.ID}, func(tx *sql.Tx) error {
		a, e = inventory.AppRow(ctx, tx, a.ID)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(a.Revision, rev); e != nil {
			return e
		}
		// The shared Tenant mutation lock also covers entity/role/App changes.
		// Validate the complete snapshot under that lock, not in an earlier read.
		if e = s.ensureSnapshotReady(ctx, tx, a.SpaceID); e != nil {
			return e
		}
		currentSnapshot, err := s.snapshot(ctx, tx, a.SpaceID)
		if err != nil {
			return err
		}
		ids, err := workspaceIDs(ctx, tx, rel.WorkspaceID)
		if err != nil {
			return err
		}
		if snapshotContractHash(remapSnapshot(currentSnapshot, ids)) != rel.SnapshotHash {
			return registry.Conflict("Workspace configuration changed after this release was captured. Capture and test a fresh release.")
		}
		if e = selectPreview(ctx, tx, a, rel.ID, origin); e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, actor, a.TenantID, "release.preview_activated", "app", a.ID, registry.Row{"release_id": rel.ID})
	})
	if e != nil {
		return nil, e
	}
	return registry.Row{"id": rel.ID, "url": origin + "/"}, nil
}

func (s *Service) deploymentAuthority(ctx context.Context, q registry.Queryer, p access.Principal, res access.Resource) error {
	var active int
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM spaces WHERE tenant_id=? AND id=? AND state='active' AND workspace_id IS NULL", res.Tenant, res.Space).Scan(&active); e != nil {
		return e
	}
	if active != 1 {
		return registry.Conflict("Select an active production Space for deployment.")
	}
	if p.Agent != nil {
		if e := access.RequireAgent(p.Agent, "production.deploy"); e != nil {
			return e
		}
	}
	for _, action := range []string{"production.deploy", "entity.publish", "app.manage", "authorization.manage"} {
		if e := s.Access.Authorize(ctx, q, p, action, res, true); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) DeployRelease(ctx context.Context, actor op.Actor, release string, revision int64) (registry.Row, error) {
	p := actor.Principal
	rel, e := s.releaseRow(ctx, s.DB, p, release)
	if e != nil {
		return nil, e
	}
	rev := revision
	if rev < 1 {
		return nil, op.RevisionRequired()
	}
	res := access.Resource{Tenant: rel.TenantID, Space: rel.ProductionSpace}
	if e = s.deploymentAuthority(ctx, s.DB, p, res); e != nil {
		return nil, e
	}
	var reserved int
	if e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM package_stages WHERE workspace_id=?", rel.WorkspaceID).Scan(&reserved); e != nil {
		return nil, e
	}
	if reserved > 0 {
		return nil, registry.Conflict("Activate the reviewed package installation from Packages.")
	}
	if e = s.EnsureReleaseBuild(ctx, s.DB, rel.ID); e != nil {
		return nil, e
	}
	record := actor.OperationID()
	ctx = context.WithValue(ctx, access.DeploymentOperationKey{}, record)
	e = s.Run.Mutate(ctx, actor, "production.deploy", res, func(tx *sql.Tx) error {
		a, e := inventory.AppRow(ctx, tx, rel.ProductionApp)
		if e != nil {
			if ae, ok := e.(*registry.APIError); !ok || ae.Status != 404 {
				return e
			}
			if rev != 1 {
				return registry.Conflict("A new production App starts at expected revision 1.")
			}
		} else {
			if e = op.CheckRevision(a.Revision, rev); e != nil {
				return e
			}
			if access.Deref(a.ActiveRelease) == rel.ID {
				return registry.Conflict("This release is already active in production.")
			}
		}
		if e = s.ensureSnapshotReady(ctx, tx, res.Space); e != nil {
			return e
		}
		current, e := s.snapshot(ctx, tx, res.Space)
		if e != nil {
			return e
		}
		if entity.Hash(current) != rel.BaseHash {
			return registry.Conflict("Production changed since this workspace was created. Create a fresh workspace and reconcile its configuration before deploying.")
		}
		var snap SpaceConfiguration
		b, _ := json.Marshal(rel.Snapshot)
		if json.Unmarshal(b, &snap) != nil || snapshotContractHash(snap) != rel.SnapshotHash {
			return registry.Conflict("Release snapshot is invalid.")
		}
		if s.HostedOrigin(rel.ProductionApp) == "" {
			return registry.Conflict("Configure the App hosting origin before deployment.")
		}
		if e = s.snapshotAppQuota(ctx, tx, res.Tenant, snap); e != nil {
			return e
		}
		// The FK target App exists before the durable deployment record is inserted.
		for _, app := range snap.Apps {
			if app.ID == rel.ProductionApp {
				if _, e = tx.ExecContext(ctx, "INSERT IGNORE INTO apps(id,tenant_id,space_id,slug,display_name,compute_mode) VALUES(?,?,?,?,?,'shared')", app.ID, res.Tenant, res.Space, app.Slug, app.Name); e != nil {
					return e
				}
			}
		}
		plan := deploymentPlan{Snapshot: snap, BaseHash: rel.BaseHash, Versions: map[string]string{}}
		for _, d := range snap.Entities {
			plan.Versions[d.ID] = registry.NewID()
		}
		pb, _ := json.Marshal(plan)
		var grantRev any
		if p.Agent != nil {
			grantRev = p.Agent.Revision
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO release_deployments(id,tenant_id,space_id,release_id,target_app_id,created_by,agent_grant_id,agent_grant_revision,plan_json) VALUES(?,?,?,?,?,?,?,?,?)", record, res.Tenant, res.Space, rel.ID, rel.ProductionApp, p.ID, agentID(p), grantRev, pb); e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, actor, res.Tenant, "release.deployment_queued", "deployment", record, registry.Row{"release_id": rel.ID, "snapshot_hash": rel.SnapshotHash})
	})
	if e != nil {
		return nil, e
	}
	return registry.Row{"id": record, "state": "queued", "instruction": "Poll deployment status. Storage preparation can pause changed entity APIs; registry security and client selection activate together. Never assume queued means deployed."}, nil
}

func (s *Service) Deployment(ctx context.Context, actor op.Actor, deployment string) (Deployment, error) {
	p := actor.Principal
	v, e := registry.ScanOne[Deployment](ctx, s.DB, "SELECT "+deploymentStatusColumns+deploymentStatusFrom+" WHERE d.id=?", deployment)
	if e != nil {
		return v, e
	}
	if e = s.Access.Authorize(ctx, s.DB, p, "release.manage", access.Resource{Tenant: v.TenantID, Space: v.SpaceID}, false); e != nil {
		return v, e
	}
	v.URL = s.HostedOrigin(v.TargetApp) + "/"
	return v, nil
}

// Recover the durable job after a browser reload or a lost deploy response.
// A pending sibling deployment also explains a shared Space's mutation fence.
func (s *Service) AppDeployment(ctx context.Context, actor op.Actor, app string) (registry.Row, error) {
	p := actor.Principal
	a, e := inventory.AppRow(ctx, s.DB, app)
	if e != nil {
		return nil, e
	}
	if e = s.Access.Authorize(ctx, s.DB, p, "release.manage", access.Resource{Tenant: a.TenantID, App: a.ID}, false); e != nil {
		return nil, e
	}
	space := a.SpaceID
	var production string
	e = s.DB.QueryRowContext(ctx, "SELECT production_space_id FROM development_workspaces WHERE backing_space_id=?", space).Scan(&production)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if production != "" {
		space = production
	}
	vs, e := registry.ScanAll[Deployment](ctx, s.DB, `SELECT `+deploymentStatusColumns+deploymentStatusFrom+`
 WHERE d.tenant_id=? AND d.space_id=? AND (d.state IN ('queued','preparing','publishing') OR d.target_app_id=? OR rel.app_id=?)
 ORDER BY (d.state IN ('queued','preparing','publishing')) DESC,d.created_at DESC,d.id DESC LIMIT 1`, a.TenantID, space, a.ID, a.ID)
	if e != nil {
		return nil, e
	}
	var deployment any // untyped nil when the App has none
	if len(vs) > 0 {
		vs[0].URL = s.HostedOrigin(vs[0].TargetApp) + "/"
		deployment = vs[0]
	}
	return registry.Row{"deployment": deployment}, nil
}

func insertRelease(ctx context.Context, tx *sql.Tx, id string, app registry.AppRecord, workspace, base, creator string, in ReleaseCreateInput, config SpaceConfiguration) (string, error) {
	snapshotJSON, _ := json.Marshal(config)
	manifestJSON, _ := json.Marshal(in.Manifest)
	hash := snapshotContractHash(config)
	_, e := tx.ExecContext(ctx, "INSERT INTO app_releases(id,tenant_id,app_id,build_id,workspace_id,display_name,manifest_json,snapshot_json,snapshot_hash,created_by,base_hash) VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, app.TenantID, app.ID, in.Build, workspace, in.Name, manifestJSON, snapshotJSON, hash, creator, base)
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO release_sources(release_id,content_hash) SELECT ?,content_hash FROM build_sources WHERE build_id=?", id, in.Build)
	return hash, e
}

func selectPreview(ctx context.Context, tx *sql.Tx, app registry.AppRecord, release, origin string) error {
	policy, e := registry.AccessOf(app.Access)
	if e != nil {
		return e
	}
	previous := policy.Origin
	policy.Origin = origin
	b, _ := json.Marshal(policy)
	_, e = tx.ExecContext(ctx, "UPDATE apps SET active_release_id=?,data_access_json=?,approved_origin=?,origin_host=NULL,origin_generation=origin_generation+IF(?<>?,1,0),revision=revision+1 WHERE id=?", release, b, origin, previous, origin, app.ID)
	return e
}
