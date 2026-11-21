package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/agent"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/authz"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/registry"
)

type releaseFixture struct {
	s                                                     *Server
	c                                                     testClient
	p                                                     access.Principal
	tenant, space, workspace, backing, app, productionApp string
	release                                               registry.Row
}

// Metadata-only workspaces suffice for management boundaries; the full release
// flow test separately provisions storage and executes real business-record APIs.
func newReleaseFixture(t *testing.T) releaseFixture {
	t.Helper()
	s, c, p, tenant, target, _ := agentSecurityFixture(t)
	ctx := context.Background()
	resource, space, productionApp := registry.NewID(), registry.NewID(), registry.NewID()
	if _, e := s.DB.Exec("INSERT INTO database_resources(id,tenant_id,target_id,database_name,management_mode,layout,state,runtime_credentials_enc) VALUES(?,?,?,'release_metadata_fixture','registered','shared','ready',X'00')", resource, tenant, target); e != nil {
		t.Fatal(e)
	}
	c.want("POST", "/v1/tenants/"+tenant+"/spaces", registry.Row{"id": space, "slug": "source", "display_name": "Source", "initial_manager_user_id": p.ID, "database": registry.Row{"resource_id": resource}}, 0, 201)
	c.want("POST", "/v1/tenants/"+tenant+"/spaces/"+space+"/apps", registry.Row{"id": productionApp, "slug": "client", "display_name": "Client", "initial_manager_user_id": p.ID}, 0, 201)
	permissions, _ := json.Marshal([]string{"read", "workspaces.write", "workspaces.test", "releases.write", "resources.write"})
	if _, e := s.DB.Exec("UPDATE agent_grants SET scope_kind='space',scope_id=?,permissions_json=?,limits_json='{}',revision=revision+1 WHERE id=?", space, permissions, p.Agent.ID); e != nil {
		t.Fatal(e)
	}
	var e error
	p.Agent, e = s.ReadAgentGrant(ctx, s.DB, p.Agent.ID)
	if e != nil {
		t.Fatal(e)
	}
	ws := c.want("POST", "/v1/spaces/"+space+"/workspaces", delivery.WorkspaceCreateInput{ID: registry.NewID(), Name: "Private human workspace"}, 0, 200)
	if _, e = s.DB.Exec("UPDATE development_workspaces SET state='ready' WHERE id=?", ws["id"]); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("UPDATE database_resources SET state='ready' WHERE id=?", ws["database_resource_id"]); e != nil {
		t.Fatal(e)
	}
	var app string
	if e = s.DB.QueryRow("SELECT development_id FROM workspace_items WHERE workspace_id=? AND kind='app' AND production_id=?", ws["id"], productionApp).Scan(&app); e != nil {
		t.Fatal(e)
	}
	s.Config.ArtifactDirectory = t.TempDir()
	ticket := c.want("POST", "/v1/apps/"+app+"/upload-tickets", registry.Row{}, 0, 201)
	upload, _ := json.Marshal(registry.Row{"entrypoint": "index.html", "files": []artifact.File{{Path: "index.html", Content: []byte("<!doctype html><h1>Release fixture</h1>")}}})
	r := httptest.NewRequest("POST", "/artifact/v1/upload", bytes.NewReader(upload))
	r.Header.Set("Authorization", "Bearer "+registry.Str(ticket["upload_token"]))
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	var build registry.Row
	if e = json.Unmarshal(w.Body.Bytes(), &build); e != nil || w.Code != 201 {
		t.Fatal("upload", w.Code, e)
	}
	rel := c.want("POST", "/v1/apps/"+app+"/releases", delivery.ReleaseCreateInput{Name: "Private snapshot", Build: registry.Str(build["id"]), Manifest: delivery.ReleaseManifest{Microapps: map[string]string{}, Dependencies: map[string]string{}}}, 1, 200)
	return releaseFixture{s, c, p, tenant, space, registry.Str(ws["id"]), registry.Str(ws["backing_space_id"]), app, productionApp, rel}
}

func TestUploadAdmissionAndRollback(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := t.Context()
	ticketPath := "/v1/apps/" + f.app + "/upload-tickets"
	ticket := f.c.want("POST", ticketPath, registry.Row{}, 0, 201)
	token := registry.Str(ticket["upload_token"])
	upload, e := f.s.delivery.AuthorizeUpload(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	files := []artifact.File{{Path: "index.html", Content: []byte("<!doctype html><h1>Uncommitted fixture</h1>")}}
	build, e := artifact.Inspect(files, "index.html")
	if e != nil {
		t.Fatal(e)
	}
	source := packageSourceArchive(t)
	assertAbsent := func() {
		t.Helper()
		for _, path := range []string{build.Hash, filepath.Join("private-source", artifact.SourceHash(source))} {
			if _, e := os.Lstat(filepath.Join(f.s.Config.ArtifactDirectory, path)); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("uncommitted artifact remains", path, e)
			}
		}
	}
	if _, e = f.s.DB.Exec(`CREATE TRIGGER fixture_reject_build BEFORE INSERT ON app_builds FOR EACH ROW
		 SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Fixture storage commit failure'`); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.delivery.StoreBuild(ctx, upload, files, build.Entry, registry.NewID(), source); e == nil {
		t.Fatal("fixture should reject the upload transaction")
	}
	assertAbsent()
	if _, e = f.s.DB.Exec("DROP TRIGGER fixture_reject_build"); e != nil {
		t.Fatal(e)
	}

	for range registry.MaxAppBuilds - 1 {
		if _, e = f.s.DB.Exec(`INSERT INTO app_builds(id,tenant_id,app_id,content_hash,files_json,entrypoint,size_bytes,created_by)
			 SELECT ?,tenant_id,app_id,content_hash,files_json,entrypoint,size_bytes,created_by FROM app_builds WHERE id=?`, registry.NewID(), f.release["build_id"]); e != nil {
			t.Fatal(e)
		}
	}
	if _, _, e = f.s.delivery.StoreBuild(ctx, upload, files, build.Entry, registry.NewID(), source); !isAPIStatus(e, 409) {
		t.Fatal("locked upload admission should reject an exhausted build quota", e)
	}
	assertAbsent()
	f.c.want("POST", ticketPath, registry.Row{}, 0, 409)
}

func TestReleaseWorkspaceOwnership(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	path := "/v1/releases/" + registry.Str(f.release["id"])
	f.c.want("GET", path, nil, 0, 200)
	read := func(p access.Principal, want int) {
		t.Helper()
		raw, _ := json.Marshal(agent.ReadInput{Resource: "release", ID: registry.Str(f.release["id"])})
		result, e := agentTool(f.s, "lastapp_get")(ctx, p, raw)
		if want == 200 {
			if e != nil || result.(delivery.Release).Snapshot.Value() == nil {
				t.Fatal("authorized release inspection", e)
			}
			return
		}
		var ae *registry.APIError
		if !errors.As(e, &ae) || ae.Status != want {
			t.Fatalf("release inspection: want %d, got %v", want, e)
		}
	}
	read(f.p, 404) // Same human, but this delegation does not own the workspace.
	var joe string
	if e := f.s.DB.QueryRow("SELECT id FROM users WHERE oidc_subject='outsider'").Scan(&joe); e != nil {
		t.Fatal(e)
	}
	f.c.want("POST", "/v1/tenants/"+f.tenant+"/memberships", registry.Row{"user_id": joe}, 0, 201)
	f.c.want("POST", "/v1/tenants/"+f.tenant+"/spaces/"+f.space+"/role-assignments", registry.Row{"id": registry.NewID(), "user_id": joe, "role_id": authz.SpaceManagerRole}, 0, 201)
	f.c.as(joe).want("GET", path, nil, 0, 404)
	if _, e := f.s.DB.Exec("UPDATE development_workspaces SET owner_agent_grant_id=? WHERE id=?", f.p.Agent.ID, f.workspace); e != nil {
		t.Fatal(e)
	}
	read(f.p, 200)
	other := f.p
	grant := registry.NewID()
	permissions, _ := json.Marshal(f.p.Agent.Permissions)
	if _, e := f.s.DB.Exec("INSERT INTO agent_grants(id,user_id,client_id,client_name,tenant_id,scope_kind,scope_id,permissions_json,limits_json,expires_at,audience_uri) VALUES(?,?,'other-client','Other client',?,'space',?,?,'{}',DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 1 DAY),?)", grant, f.p.ID, f.tenant, f.space, permissions, f.s.mcpURL()); e != nil {
		t.Fatal(e)
	}
	var e error
	other.Agent, e = f.s.ReadAgentGrant(ctx, f.s.DB, grant)
	if e != nil {
		t.Fatal(e)
	}
	read(other, 404)
	// Only a completed deployment makes a snapshot production history. A queued
	// or failed job is not permission to inspect another author's private work.
	job := registry.NewID()
	if _, e = f.s.DB.Exec("INSERT INTO release_deployments(id,tenant_id,space_id,release_id,target_app_id,created_by,plan_json) VALUES(?,?,?,?,?,?,'{}')", job, f.tenant, f.space, f.release["id"], f.productionApp, f.p.ID); e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"queued", "failed", "active"} {
		if _, e = f.s.DB.Exec("UPDATE release_deployments SET state=? WHERE id=?", state, job); e != nil {
			t.Fatal(e)
		}
		want := 404
		if state == "active" {
			want = 200
		}
		read(other, want)
		f.c.as(joe).want("GET", path, nil, 0, want)
	}
	ws := f.c.want("GET", "/v1/workspaces/"+f.workspace, nil, 0, 200)
	f.c.want("POST", "/v1/workspaces/"+f.workspace+"/archive", registry.Row{}, registry.Integer(ws["revision"]), 200)
	read(other, 200)
	f.c.as(joe).want("GET", path, nil, 0, 200)
	// Production history never authorizes the backing workspace itself.
	if e = f.s.WorkspaceGuard(ctx, f.s.DB, other, f.backing); e == nil {
		t.Fatal("release history opened a private workspace")
	}
}

func TestWorkspaceAppQuota(t *testing.T) {
	f := newReleaseFixture(t)
	if _, e := f.s.DB.Exec("UPDATE plans p JOIN tenants t ON t.plan_id=p.id SET p.max_apps=1 WHERE t.id=?", f.tenant); e != nil {
		t.Fatal(e)
	}
	create := func(space, id, slug string, want int) registry.Row {
		t.Helper()
		return f.c.want("POST", "/v1/tenants/"+f.tenant+"/spaces/"+space+"/apps", registry.Row{"id": id, "slug": slug, "display_name": slug, "initial_manager_user_id": f.p.ID}, 0, want)
	}
	create(f.space, registry.NewID(), "over-production-quota", 409)
	dev := create(f.backing, registry.NewID(), "development", 201)
	devPath := "/v1/tenants/" + f.tenant + "/apps/" + registry.Str(dev["id"])
	f.c.want("PATCH", devPath, registry.Row{"state": "archived"}, 1, 200)
	f.c.want("PATCH", devPath, registry.Row{"state": "active"}, 2, 200)
	rel := f.c.want("POST", "/v1/apps/"+f.app+"/releases", delivery.ReleaseCreateInput{Name: "Exceeds production capacity", Build: registry.Str(f.release["build_id"]), Manifest: delivery.ReleaseManifest{Microapps: map[string]string{}, Dependencies: map[string]string{}}}, 1, 200)
	rejected := f.c.want("POST", "/v1/releases/"+registry.Str(rel["id"])+"/deploy", registry.Row{}, 1, 409)
	if !strings.Contains(registry.Str(rejected), "production App quota") {
		t.Fatal("deployment rejected for wrong reason", rejected)
	}
	// The independent workspace cap still bounds all stored Apps, even archived
	// ones. Restoring one of those slots must remain possible at that cap.
	for i := 2; i < registry.MaxWorkspaceApps; i++ {
		if _, e := f.s.DB.Exec("INSERT INTO apps(id,tenant_id,space_id,slug,display_name,compute_mode) VALUES(?,?,?,?,?,'shared')", registry.NewID(), f.tenant, f.backing, fmt.Sprintf("cap-%d", i), "Cap fixture"); e != nil {
			t.Fatal(e)
		}
	}
	create(f.backing, registry.NewID(), "over-workspace-cap", 409)
	f.c.want("PATCH", devPath, registry.Row{"state": "archived"}, 3, 200)
	f.c.want("PATCH", devPath, registry.Row{"state": "active"}, 4, 200)
	prodPath := "/v1/tenants/" + f.tenant + "/apps/" + f.productionApp
	f.c.want("PATCH", prodPath, registry.Row{"state": "archived"}, 1, 200)
	create(f.space, registry.NewID(), "replacement", 201)
	f.c.want("PATCH", prodPath, registry.Row{"state": "active"}, 2, 409)
}

func TestReleasePreviewConcurrentConfiguration(t *testing.T) {
	f := newReleaseFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, e := f.s.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = registry.LockTenant(ctx, tx, f.tenant); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "/v1/releases/"+registry.Str(f.release["id"])+"/preview", strings.NewReader("{}"))
	r = r.WithContext(ctx)
	r.SetPathValue("release", registry.Str(f.release["id"]))
	r.Header.Set("If-Match", "1")
	done := make(chan error, 1)
	go func() { done <- f.s.releasePreview(httptest.NewRecorder(), r, access.Principal{ID: f.p.ID}) }()
	// Wait for the real preview transaction to contend on the Tenant lock. This
	// puts the concurrent change after any unlocked preflight in the old code.
	for {
		var waiting int
		e = f.s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE DB=DATABASE() AND ID<>CONNECTION_ID() AND LOWER(INFO) LIKE '%select id from tenants%' AND LOWER(INFO) LIKE '%for update%'").Scan(&waiting)
		if e != nil {
			t.Fatal(e)
		}
		if waiting > 0 {
			break
		}
		select {
		case early := <-done:
			t.Fatalf("preview did not wait for concurrent mutation: %v", early)
		case <-ctx.Done():
			t.Fatal("preview did not reach the Tenant lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO roles(id,tenant_id,space_id,owner_key,scope_kind,domain,code,display_name) VALUES(?,?,?,?,'space','data','new-role','Concurrent role')", registry.NewID(), f.tenant, f.backing, "s:"+f.backing); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
	case <-ctx.Done():
		t.Fatal("preview did not resume")
	}
	var ae *registry.APIError
	if !errors.As(e, &ae) || ae.Status != 409 || !strings.Contains(ae.Message, "configuration changed") {
		t.Fatalf("preview accepted a changed snapshot or failed for the wrong reason: %v", e)
	}
	var active int
	if e = f.s.DB.QueryRow("SELECT COUNT(*) FROM apps WHERE id=? AND active_release_id IS NOT NULL", f.app).Scan(&active); e != nil || active != 0 {
		t.Fatal("rejected preview changed client selection", active, e)
	}
}
