package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/inventory"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

func (s *Service) ownedDevelopmentApp(ctx context.Context, q registry.Queryer, p access.Principal, app string) (registry.AppRecord, registry.Workspace, error) {
	a, e := inventory.AppRow(ctx, q, app)
	if e != nil {
		return registry.AppRecord{}, registry.Workspace{}, e
	}
	if a.State != registry.StateActive {
		return registry.AppRecord{}, registry.Workspace{}, registry.Conflict("Restore the App before developing or releasing it.")
	}
	w, e := registry.ScanOne[registry.Workspace](ctx, q, "SELECT "+registry.WorkspaceColumns+" FROM development_workspaces w WHERE w.backing_space_id=? AND w.state='ready'", a.SpaceID)
	if e != nil {
		return registry.AppRecord{}, registry.Workspace{}, registry.Missing()
	}
	if e = s.Access.WorkspaceOwner(ctx, q, p, w); e != nil {
		return registry.AppRecord{}, registry.Workspace{}, e
	}
	if e = s.Access.Authorize(ctx, q, p, "release.manage", access.Resource{Tenant: a.TenantID, App: app}, true); e != nil {
		return registry.AppRecord{}, registry.Workspace{}, e
	}
	return a, w, nil
}

func (s *Service) UploadTicket(ctx context.Context, actor op.Actor, app string) (registry.Row, error) {
	p := actor.Principal
	if s.ArtifactDirectory() == "" {
		return nil, registry.Conflict("Configure persistent artifact storage first.")
	}
	a, _, e := s.ownedDevelopmentApp(ctx, s.DB, p, app)
	if e != nil {
		return nil, e
	}
	if p.Agent != nil {
		if e = access.RequireAgent(p.Agent, "releases.write"); e != nil {
			return nil, e
		}
	}
	secret := registry.Token(32)
	e = s.Run.Mutate(ctx, actor, "release.manage", access.Resource{Tenant: a.TenantID, App: a.ID}, func(tx *sql.Tx) error {
		if e := checkAppBuildQuota(ctx, tx, a.ID); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM artifact_uploads WHERE user_id=? AND expires_at>UTC_TIMESTAMP(6) AND build_id IS NULL", p.ID).Scan(&n); e != nil {
			return e
		}
		if n >= registry.MaxOutstandingUploads {
			return registry.Conflict("Too many outstanding uploads. Finish an upload or wait for expiry.")
		}
		var rev any
		if p.Agent != nil {
			rev = p.Agent.Revision
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO artifact_uploads(token_hash,tenant_id,app_id,user_id,agent_grant_id,agent_grant_revision,expires_at) VALUES(?,?,?,?,?,?,?)", registry.TokenHash(secret), a.TenantID, a.ID, p.ID, agentID(p), rev, time.Now().Add(registry.UploadTicketLifetime))
		return e
	})
	if e != nil {
		return nil, e
	}
	return registry.Row{"upload_url": s.Config.PublicURL + "/artifact/v1/upload", "upload_token": secret, "expires_in": int(registry.UploadTicketLifetime.Seconds()), "limits": artifact.Limits, "instructions": "POST {entrypoint:'index.html',files:[{path:'...',content_base64:'...'}]} using Authorization: Bearer <upload_token>. A ticket uploads one immutable build; identical retries return its build ID. Do not put tokens or local development credentials in builds."}, nil
}

// uploadTicket is a bounded build-upload credential and the build it
// produced, if it was already used.
type uploadTicket struct {
	TenantID, AppID, UserID, AgentGrant string // AgentGrant is empty for a person
	AgentRevision                       int64
	BuildID                             *string
}

const uploadTicketColumns = "tenant_id,app_id,user_id,COALESCE(agent_grant_id,''),COALESCE(agent_grant_revision,0),build_id"

func (v *uploadTicket) ScanFields() []any {
	return []any{&v.TenantID, &v.AppID, &v.UserID, &v.AgentGrant, &v.AgentRevision, &v.BuildID}
}

// storedBuild is an uploaded static build and its file inventory.
type storedBuild struct {
	ID, Hash, Entry string
	Size            int64
	Files           registry.JSONValue
}

const storedBuildColumns = "b.id,b.content_hash,b.entrypoint,b.size_bytes,b.files_json"

func (v *storedBuild) ScanFields() []any { return []any{&v.ID, &v.Hash, &v.Entry, &v.Size, &v.Files} }

// files decodes the build's file inventory.
func (v storedBuild) files() ([]artifact.FileInfo, error) {
	var files []artifact.FileInfo
	b, _ := json.Marshal(v.Files)
	return files, json.Unmarshal(b, &files)
}

// Build is an uploaded build in an App's collection.
type Build struct {
	Hash      string    `json:"content_hash"`
	CreatedAt time.Time `json:"created_at"`
	Entry     string    `json:"entrypoint"`
	ID        string    `json:"id"`
	Size      int64     `json:"size_bytes"`
}

func (v *Build) ScanFields() []any { return []any{&v.Hash, &v.CreatedAt, &v.Entry, &v.ID, &v.Size} }

func (s *Service) uploadPrincipal(ctx context.Context, q registry.Queryer, v uploadTicket) (access.Principal, error) {
	p := access.Principal{ID: v.UserID}
	if v.AgentGrant != "" {
		g, e := s.Access.ReadAgentGrant(ctx, q, v.AgentGrant)
		if e != nil {
			return p, e
		}
		if g.Revision != v.AgentRevision {
			return p, registry.Missing()
		}
		if e = access.RequireAgent(g, "releases.write"); e != nil {
			return p, e
		}
		p.Agent = g
	}
	if _, _, e := s.ownedDevelopmentApp(ctx, q, p, v.AppID); e != nil {
		return p, e
	}
	return p, nil
}

func (s *Service) Builds(ctx context.Context, actor op.Actor, app string, page op.PageRequest) (op.Page[Build], error) {
	p := actor.Principal
	a, e := inventory.AppRow(ctx, s.DB, app)
	if e != nil {
		return op.Page[Build]{}, e
	}
	if e = s.Access.Authorize(ctx, s.DB, p, "release.manage", access.Resource{Tenant: a.TenantID, App: a.ID}, false); e != nil {
		return op.Page[Build]{}, e
	}
	vs, e := op.PageScan[Build](ctx, s.DB, page, op.CollectionFor("builds"), "SELECT content_hash,created_at,entrypoint,id,size_bytes FROM app_builds WHERE app_id=? ORDER BY created_at DESC,id", a.ID)
	return vs, e
}

func (s *Service) EnsureReleaseBuild(ctx context.Context, q registry.Queryer, release string) error {
	v, e := registry.ScanOne[storedBuild](ctx, q, "SELECT "+storedBuildColumns+" FROM app_releases rel JOIN app_builds b ON b.id=rel.build_id WHERE rel.id=?", release)
	if e != nil {
		return e
	}
	files, e := v.files()
	if e != nil {
		return e
	}
	build := artifact.Build{Hash: v.Hash, Files: files, Size: int(v.Size), Entry: v.Entry}
	if e = artifact.Check(s.ArtifactDirectory(), build); e != nil {
		return registry.Conflict("Release files are unavailable or corrupt on this server. Restore the shared artifact storage before deploying.")
	}
	return nil
}

// Upload is an upload ticket whose live authority AuthorizeUpload checked.
type Upload struct {
	raw    string
	ticket uploadTicket
}

// AuthorizeUpload checks a raw upload ticket and the live authority of the
// person or delegated agent it was issued to, before the body is read.
func (s *Service) AuthorizeUpload(ctx context.Context, raw string) (Upload, error) {
	v, e := registry.ScanOne[uploadTicket](ctx, s.DB, "SELECT "+uploadTicketColumns+" FROM artifact_uploads WHERE token_hash=? AND expires_at>UTC_TIMESTAMP(6)", registry.TokenHash(raw))
	if e != nil {
		return Upload{}, registry.Problem(401, "invalid_upload", "Upload ticket expired or invalid.")
	}
	if _, e = s.uploadPrincipal(ctx, s.DB, v); e != nil {
		return Upload{}, e
	}
	if v.BuildID == nil {
		if e = checkAppBuildQuota(ctx, s.DB, v.AppID); e != nil {
			return Upload{}, e
		}
	}
	return Upload{raw: raw, ticket: v}, nil
}

// StoreBuild stores one immutable build for an authorized upload ticket. A
// ticket produces one build: an identical retry returns it (created is false),
// other content is rejected. requestID attributes the audit event.
func (s *Service) StoreBuild(ctx context.Context, u Upload, in []artifact.File, entry, requestID string, source ...[]byte) (out registry.Row, created bool, err error) {
	raw, v := u.raw, u.ticket
	b, e := artifact.Inspect(in, entry)
	if e != nil {
		return nil, false, registry.Bad(e.Error())
	}
	var sourceHash string
	if len(source) > 0 && len(source[0]) > 0 {
		if len(source[0]) > artifact.SourceLimit {
			return nil, false, registry.Bad("Source archive exceeds its byte limit.")
		}
		sourceHash = artifact.SourceHash(source[0])
	}
	unlock, e := s.LockArtifacts(ctx, b.Hash, sourceHash)
	if e != nil {
		return nil, false, e
	}
	defer unlock()
	var persistedBuild, persistedSource bool
	defer func() {
		if !created && (persistedBuild || persistedSource) {
			s.reclaimUpload(ctx, v.TenantID, b.Hash, persistedBuild, sourceHash, persistedSource)
		}
	}()

	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, false, e
	}
	defer tx.Rollback()
	if e = registry.LockTenant(ctx, tx, v.TenantID); e != nil {
		return nil, false, e
	}
	v, e = registry.ScanOne[uploadTicket](ctx, tx, "SELECT "+uploadTicketColumns+" FROM artifact_uploads WHERE token_hash=? AND expires_at>UTC_TIMESTAMP(6) FOR UPDATE", registry.TokenHash(raw))
	if e != nil {
		return nil, false, registry.Missing()
	}
	p, e := s.uploadPrincipal(ctx, tx, v)
	if e != nil {
		return nil, false, e
	}
	if v.BuildID != nil {
		existing, e := buildReplay(ctx, tx, *v.BuildID, b.Hash, sourceHash)
		return existing, false, e
	}
	if e = checkAppBuildQuota(ctx, tx, v.AppID); e != nil {
		return nil, false, e
	}
	if sourceHash != "" {
		if _, e = artifact.StoreSource(s.ArtifactDirectory(), source[0]); e != nil {
			return nil, false, registry.Bad("Invalid source archive or source storage unavailable.")
		}
		persistedSource = true
	}
	if e = artifact.Store(s.ArtifactDirectory(), in, b); e != nil {
		s.Warn(ctx, "artifact storage failed", e)
		return nil, false, registry.Problem(503, "artifact_storage", "Could not persist the build.")
	}
	persistedBuild = true

	build := registry.NewID()
	if e = insertBuild(ctx, tx, build, v.TenantID, v.AppID, p.ID, b, sourceHash); e != nil {
		return nil, false, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE artifact_uploads SET build_id=? WHERE token_hash=?", build, registry.TokenHash(raw)); e != nil {
		return nil, false, e
	}
	ctx = op.AgentAuditContext(ctx, v.AgentGrant, "")
	if e = op.Audit(ctx, tx, v.TenantID, p.ID, "build.uploaded", "app", v.AppID, requestID, registry.Row{"build_id": build, "content_hash": b.Hash, "size_bytes": b.Size}); e != nil {
		return nil, false, e
	}
	if e = tx.Commit(); e != nil {
		return nil, false, e
	}
	return registry.Row{"id": build, "content_hash": b.Hash, "size_bytes": b.Size}, true, nil
}

func checkAppBuildQuota(ctx context.Context, q registry.Queryer, app string) error {
	var n int
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_builds WHERE app_id=?", app).Scan(&n); e != nil {
		return e
	}
	if n >= registry.MaxAppBuilds {
		return registry.Conflict("App build limit reached. Create a fresh workspace.")
	}
	return nil
}

// HostedApp resolves an active App on its static host: whether it belongs to
// a ready development workspace, and its selected release (nil if none).
func (s *Service) HostedApp(ctx context.Context, app string) (development bool, release *string, err error) {
	var workspace *string
	err = registry.Lookup(ctx, s.DB, "SELECT sp.workspace_id,a.active_release_id FROM apps a JOIN spaces sp ON sp.id=a.space_id AND sp.state='active' JOIN tenants t ON t.id=a.tenant_id AND t.state='active' LEFT JOIN development_workspaces ws ON ws.id=sp.workspace_id WHERE a.id=? AND a.state='active' AND (sp.workspace_id IS NULL OR ws.state='ready')", []any{app}, &workspace, &release)
	return workspace != nil, release, err
}

// ReleaseEntry returns the build and entrypoint of a release.
func (s *Service) ReleaseEntry(ctx context.Context, release string) (build, entry string, err error) {
	err = registry.Lookup(ctx, s.DB, "SELECT b.id,b.entrypoint FROM app_releases r JOIN app_builds b ON b.id=r.build_id WHERE r.id=?", []any{release}, &build, &entry)
	return build, entry, err
}

// HostedFile opens one file of a build served on an App's host, with its
// size. Historical builds remain addressable for already-open clients, but
// only if a release was actually deployed to this App (never arbitrary
// workspace builds). The caller closes the file.
func (s *Service) HostedFile(ctx context.Context, app, id, name string) (io.ReadCloser, int, error) {
	build, e := registry.ScanOne[storedBuild](ctx, s.DB, "SELECT "+storedBuildColumns+" FROM app_builds b WHERE b.id=? AND (EXISTS(SELECT 1 FROM app_releases rel WHERE rel.build_id=b.id AND rel.app_id=?) OR EXISTS(SELECT 1 FROM release_deployments d JOIN app_releases rel ON rel.id=d.release_id WHERE rel.build_id=b.id AND d.target_app_id=? AND d.state='active'))", id, app, app)
	if e != nil {
		return nil, 0, e
	}
	files, e := build.files()
	if e != nil {
		return nil, 0, e
	}
	allowed := false
	size := 0
	for _, f := range files {
		if f.Path == name {
			allowed = true
			size = f.Size
			break
		}
	}
	if !allowed {
		return nil, 0, registry.Missing()
	}
	file, e := artifact.Read(s.ArtifactDirectory(), build.Hash, name)
	return file, size, e
}

func buildReplay(ctx context.Context, q registry.Queryer, id, hash, source string) (registry.Row, error) {
	var oldHash, oldSource string
	e := registry.Lookup(ctx, q, `SELECT b.content_hash,COALESCE(bs.content_hash,'') FROM app_builds b
	 LEFT JOIN build_sources bs ON bs.build_id=b.id WHERE b.id=?`, []any{id}, &oldHash, &oldSource)
	if e != nil {
		return nil, e
	}
	if oldHash != hash || oldSource != source {
		return nil, registry.Conflict("Upload ticket already used for different content.")
	}
	return registry.Row{"content_hash": oldHash, "id": id}, nil
}

func insertBuild(ctx context.Context, tx *sql.Tx, id, tenant, app, creator string, b artifact.Build, source string) error {
	files, _ := json.Marshal(b.Files)
	_, e := tx.ExecContext(ctx, "INSERT INTO app_builds(id,tenant_id,app_id,content_hash,files_json,entrypoint,size_bytes,created_by) VALUES(?,?,?,?,?,?,?,?)", id, tenant, app, b.Hash, files, b.Entry, b.Size, creator)
	if e != nil || source == "" {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO build_sources(build_id,content_hash) VALUES(?,?)", id, source)
	return e
}
