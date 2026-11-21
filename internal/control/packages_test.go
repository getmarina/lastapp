package control

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	mariadb "github.com/aantich/go-mariadb"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aantich/lastapp/internal/agent"
	"github.com/aantich/lastapp/internal/apppackages"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/authz"
	"github.com/aantich/lastapp/internal/databases"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/testdb"
)

func TestPackageInstallation(t *testing.T) {
	s, c, _, owner, member := foundationFixture(t)
	s.Config.ArtifactDirectory = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	admin := testdb.Load(t, testdb.ConfigEnv).Admin
	host, port := registryAddress(admin.RegistryDSN)
	dsn, e := mariadb.ParseDSN(admin.RegistryDSN)
	if e != nil {
		t.Fatal(e)
	}
	target := registry.NewID()
	enc, e := s.sealer.Seal("target", target, "", databases.Credentials{Username: dsn.User, Password: dsn.Passwd})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("INSERT INTO database_targets(id,display_name,canonical_host,port,tls_mode,managed_enabled,provisioning_credentials_enc) VALUES(?,'Packages fixture',?,?,'disabled',1,?)", target, host, port, enc); e != nil {
		t.Fatal(e)
	}

	source := packageSourceArchive(t)
	files := []artifact.File{{Path: "index.html", Content: []byte("<!doctype html><p>Fixture App</p>")}}
	build, e := artifact.Inspect(files, "index.html")
	if e != nil {
		t.Fatal(e)
	}
	def := entity.Templates()[0]
	def.Permissions = []entity.Permission{
		{Key: "read", Label: "Read", Operation: "read", Roles: []string{"reader"}, Fields: []string{"name"}},
		{Key: "create", Label: "Create", Operation: "create", Roles: []string{"reader"}, Fields: []string{"name"}},
	}
	manifest := apppackages.Manifest{Format: apppackages.Format, Key: "sample", Version: "1.0.0", Name: "Fixture package", Definitions: []entity.Definition{def}, Roles: []apppackages.Role{{Key: "reader", Name: "Reader"}}, App: &apppackages.App{
		Key: "sample", Name: "Fixture App", Access: entity.AppAccess{Ceiling: []entity.Gate{{Permission: def.Key + "/read", Fields: []string{"name"}}}},
		Microapps: []apppackages.Microapp{{Key: "records", Name: "Records", Version: "1.0.0"}, {Key: "overview", Name: "Overview", Version: "1.0.0"}},
	}}
	publish := func(m apppackages.Manifest, b *artifact.Build, f []artifact.File, src []byte) apppackages.Version {
		t.Helper()
		raw, e := json.Marshal(m)
		if e != nil {
			t.Fatal(e)
		}
		v, e := s.PublishPackage(ctx, bytes.NewReader(raw), b, f, src)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	v := publish(manifest, &build, files, source)
	if other := publish(manifest, &build, files, source); other != v {
		t.Fatal(other, v)
	}
	prereq := entity.Templates()[1]
	prereq.Fields = append(prereq.Fields, entity.Field{Key: "company", Label: "Company", Type: "reference", Target: "sample/company"})
	publish(apppackages.Manifest{Format: 1, Key: "related", Version: "1.0.0", Name: "Related package", Dependencies: map[string]string{"sample": "1.0.0"}, Definitions: []entity.Definition{prereq}}, nil, nil, nil)
	done := make(chan struct{})
	go func() { defer close(done); s.RunWorker(ctx) }()
	defer func() { cancel(); <-done }()
	oc := c.as(owner)
	wait := func(path, state, key string) registry.Row {
		t.Helper()
		deadline := time.Now().Add(35 * time.Second)
		for time.Now().Before(deadline) {
			row := oc.want("GET", path, nil, 0, 200)
			if row[key] == state {
				return row
			}
			if row[key] == "failed" {
				t.Fatalf("waiting for %s: %v", state, row)
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("%s did not reach %s", path, state)
		return nil
	}
	for index, mode := range []string{apppackages.Objects, apppackages.Full} {
		tenant, space := registry.NewID(), registry.NewID()
		base := "/v1/tenants/" + tenant
		planID := "10000000-0000-4000-8000-000000000001"
		if mode == apppackages.Objects {
			planID = "10000000-0000-4000-8000-000000000002"
		}
		c.want("POST", "/admin/v1/tenants", registry.Row{"id": tenant, "slug": "pack-" + tenant, "display_name": "Package fixture", "plan_id": planID, "initial_admin_user_id": owner}, 0, 201)
		oc.want("POST", base+"/memberships", registry.Row{"user_id": member}, 0, 201)
		created := oc.want("POST", base+"/spaces", registry.Row{"id": space, "slug": "packages", "display_name": "Packages", "initial_manager_user_id": owner, "database": registry.Row{"mode": "managed", "target_id": target}}, 0, 201)
		r := registry.Str(created["database_resource_id"])
		if r == "" {
			if e = s.DB.QueryRow("SELECT database_resource_id FROM spaces WHERE id=?", space).Scan(&r); e != nil {
				t.Fatal(e)
			}
		}
		wait(base+"/database-resources/"+r, "ready", "state")
		oc.want("GET", base+"/packages", nil, 0, 200)
		var sibling string
		stage := func(key, mode string) (id, path string) {
			t.Helper()
			sel := registry.Row{"key": key, "version": "1.0.0", "space_id": space, "mode": mode}
			plan := oc.want("POST", base+"/package-plans", sel, 0, 200)
			twin := oc.want("POST", base+"/package-plans", sel, 0, 200)
			if plan["plan_hash"] != twin["plan_hash"] {
				t.Fatal("unstable plan")
			}
			sel["plan_hash"], sel["request_key"] = plan["plan_hash"], registry.NewID()
			staged := oc.want("POST", base+"/package-stages", sel, 0, 202)
			again := oc.want("POST", base+"/package-stages", sel, 0, 202)
			if staged["workspace_id"] != again["workspace_id"] {
				t.Fatal("stage replay created another workspace")
			}
			id = registry.Str(staged["workspace_id"])
			path = base + "/package-stages/" + id
			wait(path, "ready", "state")
			return id, path
		}
		install := func(key, mode string) registry.Row {
			t.Helper()
			id, path := stage(key, mode)
			if mode != apppackages.Objects {
				oc.want("POST", path+"/preview", nil, 0, 200)
			}
			fault := key == "sample" && mode != apppackages.Attach
			if fault {
				_, e := s.DB.Exec(`CREATE TRIGGER fixture_package_activation_failure BEFORE UPDATE ON entity_versions FOR EACH ROW BEGIN
				 IF NEW.state='published' AND NEW.deployment_id IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Fixture activation failure'; END IF;
				END`)
				if e != nil {
					t.Fatal(e)
				}
			}
			oc.want("POST", path+"/activate", nil, 0, 200)
			if fault {
				failed := wait(path, "failed", "state")
				if failed["failed_step"] != "publishing" {
					t.Fatal(failed)
				}
				if _, e = s.DB.Exec("DROP TRIGGER fixture_package_activation_failure"); e != nil {
					t.Fatal(e)
				}
				version := int64(failed["revision"].(float64))
				recovery := oc
				if mode == apppackages.Objects {
					oc.want("PATCH", base+"/apps/"+sibling, registry.Row{"display_name": "Updated sibling"}, 2, 200)
				} else {
					archivePackageWorkspace(oc, id)
					recovery = c.as(member)
					recovery.want("POST", path+"/retry", nil, version, 403)
					oc.want("POST", base+"/spaces/"+space+"/role-assignments", registry.Row{"id": registry.NewID(), "user_id": member, "role_id": authz.SpaceManagerRole}, 0, 201)
					recovery.want("GET", "/v1/releases/"+registry.Str(failed["release_id"]), nil, 0, 404)
					if _, e = s.DB.Exec("UPDATE release_deployments SET failed_base_hash=NULL WHERE id=?", failed["operation_id"]); e != nil {
						t.Fatal(e)
					}
				}
				recovery.want("POST", path+"/retry", nil, version, 409)
				review := recovery.want("GET", path+"/retry-plan", nil, 0, 200)
				recovery.want("POST", path+"/retry?base_hash="+registry.Str(review["base_hash"]), nil, version, 200)
			}
			result := wait(path, "active", "state")
			oc.want("GET", base+"/package-stages", nil, 0, 200)
			return result
		}
		if index == 0 {
			id, path := stage("sample", apppackages.Objects)
			if _, e = s.DB.Exec("UPDATE database_targets SET state='disabled' WHERE id=?", target); e != nil {
				t.Fatal(e)
			}
			ws := archivePackageWorkspace(oc, id)
			oc.want("POST", path+"/discard", nil, 0, 200)
			oc.want("POST", base+"/package-plans", registry.Row{"key": "sample", "version": "1.0.0", "space_id": space, "mode": mode}, 0, 409)
			if tenant, progressed, e := s.provisioner.PreviewCleanupJob(ctx, s.DB, ""); e != nil || progressed || tenant != "" {
				t.Fatal("preview cleanup should wait for its disabled target", progressed, e)
			}
			if _, e = s.DB.Exec("UPDATE database_targets SET state='active' WHERE id=?", target); e != nil {
				t.Fatal(e)
			}
			database := wait(base+"/database-resources/"+registry.Str(ws["database_resource_id"]), "retired", "state")
			var remaining int
			if e = s.DB.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=?", database["database_name"]).Scan(&remaining); e != nil || remaining != 0 {
				t.Fatal("discard did not reclaim physical preview storage", remaining, e)
			}
			sibling = registry.NewID()
			oc.want("POST", base+"/spaces/"+space+"/apps", registry.Row{"id": sibling, "slug": "existing", "display_name": "Existing App", "initial_manager_user_id": owner}, 0, 201)
			oc.want("PATCH", "/v1/apps/"+sibling+"/data-access", entity.AppAccess{Origin: "http://localhost:5390", Inherit: true}, 1, 200)
		}
		result := install("sample", mode)
		if index == 0 {
			if result["app_id"] != nil {
				t.Fatal("objects-only install created an App", result)
			}
			var role, eid string
			if e = s.DB.QueryRow("SELECT id FROM roles WHERE space_id=? AND code='reader'", space).Scan(&role); e != nil {
				t.Fatal(e)
			}
			oc.want("POST", base+"/spaces/"+space+"/role-assignments", registry.Row{"id": registry.NewID(), "role_id": role, "user_id": owner}, 0, 201)
			token := packageDataSession(t, oc, sibling, "http://localhost:5390")
			status, body := packageData(t, oc, "http://localhost:5390", token, "POST", "/entities/company/records", registry.Row{"id": registry.NewID(), "values": registry.Row{"name": "Kept through App attachment"}})
			if status != 200 {
				t.Fatal(status, body)
			}
			if e = s.DB.QueryRow("SELECT id FROM entity_definitions WHERE space_id=? AND entity_key='company'", space).Scan(&eid); e != nil {
				t.Fatal(e)
			}
			detail := oc.want("GET", "/v1/entities/"+eid, nil, 0, 200)
			var updated entity.Definition
			raw, _ := json.Marshal(detail["draft_json"])
			if updated, e = entity.Decode(raw); e != nil {
				t.Fatal(e)
			}
			updated.Fields = append(updated.Fields, entity.Field{Key: "notes", Label: "Notes", Type: "text", MaxLength: 80})
			saved := oc.want("PATCH", "/v1/entities/"+eid, registry.Row{"definition": updated}, int64(detail["revision"].(float64)), 200)
			oc.want("POST", "/v1/entities/"+eid+"/publications", registry.Row{}, int64(saved["revision"].(float64)), 202)
			deadline := time.Now().Add(10 * time.Second)
			for {
				var pending int
				if e = s.DB.QueryRow("SELECT COUNT(*) FROM entity_definitions WHERE id=? AND pending_version_id IS NOT NULL", eid).Scan(&pending); e != nil {
					t.Fatal(e)
				}
				if pending == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("edited object did not publish")
				}
				time.Sleep(100 * time.Millisecond)
			}
			packageAgentInstall(t, s, c, owner, tenant, space)
			result = install("sample", apppackages.Attach)
			launch := oc.want("GET", "/v1/apps/"+registry.Str(result["app_id"])+"/launch-info", nil, 0, 200)
			origin := registry.Str(launch["origin"])
			token = packageDataSession(t, oc, registry.Str(result["app_id"]), origin)
			status, body = packageData(t, oc, origin, token, "POST", "/entities/company/query", registry.Row{"select": []string{"name"}, "include_count": true})
			if status != 200 || body["count"].(float64) != 1 {
				t.Fatal(status, body)
			}
			current := oc.want("GET", base+"/apps/"+sibling, nil, 0, 200)
			if current["revision"].(float64) != 3 {
				t.Fatal("package modified a sibling App", current)
			}
		}
		if registry.Str(result["app_id"]) == "" || registry.Str(result["release_id"]) == "" {
			t.Fatal(result)
		}
		if mode == apppackages.Full {
			checkPackageAgentReceiptsAndSource(t, s, c, owner, tenant, result)
		}
		oc.want("GET", base+"/my-apps", nil, 0, 200)
		memberClient := c.as(member)
		empty := memberClient.want("GET", base+"/my-apps", nil, 0, 200)
		if len(empty["items"].([]any)) != 0 {
			t.Fatal(empty)
		}
		var rid string
		if e = s.DB.QueryRow("SELECT id FROM roles WHERE space_id=? AND code='reader'", space).Scan(&rid); e != nil {
			t.Fatal(e)
		}
		oc.want("POST", base+"/spaces/"+space+"/role-assignments", registry.Row{"id": registry.NewID(), "role_id": rid, "user_id": member}, 0, 201)
		launch := memberClient.want("GET", base+"/my-apps", nil, 0, 200)
		if len(launch["items"].([]any)) != 1 {
			t.Fatal(launch)
		}
	}
	orphanFiles := []artifact.File{{Path: "index.html", Content: []byte("<p>Abandoned bundle</p>")}}
	orphan, e := artifact.Inspect(orphanFiles, "index.html")
	if e != nil {
		t.Fatal(e)
	}
	if e = artifact.Store(s.Config.ArtifactDirectory, orphanFiles, orphan); e != nil {
		t.Fatal(e)
	}
	before := time.Now().Add(-48 * time.Hour)
	for _, b := range []artifact.Build{orphan, build} {
		if e = os.Chtimes(filepath.Join(s.Config.ArtifactDirectory, b.Hash), before, before); e != nil {
			t.Fatal(e)
		}
	}
	removed, e := s.PrunePackageArtifacts(ctx)
	if e != nil || removed != 1 {
		t.Fatal("artifact retention", removed, e)
	}
	if e = artifact.Check(s.Config.ArtifactDirectory, build); e != nil {
		t.Fatal("published artifact was removed", e)
	}
}

func packageSourceArchive(t *testing.T) []byte {
	t.Helper()
	var source bytes.Buffer
	z := zip.NewWriter(&source)
	for _, name := range []string{"package.json", "package-lock.json", "src/main.js"} {
		f, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = io.WriteString(f, "{}"); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return source.Bytes()
}

func checkPackageAgentReceiptsAndSource(t *testing.T, s *Server, c testClient, owner, tenant string, stage registry.Row) {
	t.Helper()
	ctx := t.Context()
	token, _ := goldenGrant(t, s, owner, "tenant", tenant, "read", "releases.write", "production.deploy")
	path := "/mcp/sources/" + registry.Str(stage["release_id"])
	r := httptest.NewRequest("GET", path, nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+token)
	p, e := s.agent.Principal(r)
	if e != nil {
		t.Fatal(e)
	}
	tool := agentTool(s, "lastapp_package")
	for action, id := range map[string]any{"activate": stage["operation_id"], "preview": stage["workspace_id"]} {
		in := agent.PackageWriteInput{Action: action, Stage: registry.Str(stage["workspace_id"]), RequestKey: registry.NewID()}
		if _, e = tool(ctx, p, agentTestJSON(t, in)); e != nil {
			t.Fatal(action, e)
		}
		replay, e := tool(ctx, p, agentTestJSON(t, in))
		if e != nil {
			t.Fatal(action, e)
		}
		row, ok := replay.(registry.Row)
		if !ok || row["replayed"] != true || row["resource"].(registry.Row)["id"] != id {
			t.Fatalf("%s did not replay the original resource: %v", action, replay)
		}
		in.Revision++
		if _, e = tool(ctx, p, agentTestJSON(t, in)); !isAPIStatus(e, 409) {
			t.Fatalf("%s key accepted different arguments: %v", action, e)
		}
	}
	w := &packageDownloadRecorder{ResponseRecorder: httptest.NewRecorder(), agent: s.agent}
	c.h.ServeHTTP(w, r)
	if w.Code != 200 || !w.isolated {
		t.Fatal("source should stream in its own slot", w.Code, w.isolated)
	}
}

type packageDownloadRecorder struct {
	*httptest.ResponseRecorder
	agent    *agent.Server
	isolated bool
}

func (w *packageDownloadRecorder) Write(b []byte) (int, error) {
	w.isolated = len(w.agent.ValidationSlots) == 0 && len(w.agent.SourceSlots) == 1
	return w.ResponseRecorder.Write(b)
}

func archivePackageWorkspace(c testClient, id string) registry.Row {
	c.t.Helper()
	path := "/v1/workspaces/" + id
	ws := c.want("GET", path, nil, 0, 200)
	c.want("POST", path+"/archive", nil, registry.Integer(ws["revision"]), 200)
	return ws
}

func packageData(t *testing.T, c testClient, origin, token, method, path string, body any) (int, registry.Row) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "/data/v1"+path, bytes.NewReader(raw))
	r.Header.Set("Origin", origin)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	var out registry.Row
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e, w.Code)
	}
	return w.Code, out
}

func packageDataSession(t *testing.T, c testClient, app, origin string) string {
	t.Helper()
	verifier, nonce := registry.Token(32), registry.Token(32)
	code := c.want("POST", "/v1/apps/"+app+"/launch", registry.Row{"challenge": base64.RawURLEncoding.EncodeToString(registry.TokenHash(verifier)), "nonce": nonce}, 0, 200)
	status, out := packageData(t, c, origin, "", "POST", "/session/exchange", registry.Row{"code": code["code"], "verifier": verifier, "nonce": nonce})
	if status != 200 {
		t.Fatal(status, out)
	}
	return registry.Str(out["access_token"])
}

func packageAgentInstall(t *testing.T, s *Server, c testClient, owner, tenant, space string) {
	t.Helper()
	ctx := t.Context()
	token, _ := goldenGrant(t, s, owner, "tenant", tenant, "read", "workspaces.write", "production.deploy")
	server := httptest.NewServer(c.h)
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "package-fixture", Version: "1"}, nil)
	session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: agentRoundTripper{token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	call := func(name string, in registry.Row) registry.Row {
		t.Helper()
		result, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: in})
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var resultRow struct {
			OK   bool         `json:"ok"`
			Data registry.Row `json:"data"`
		}
		if e = json.Unmarshal(raw, &resultRow); e != nil || !resultRow.OK {
			t.Fatalf("%s: %s", name, raw)
		}
		return resultRow.Data
	}
	sel := registry.Row{"key": "related", "version": "1.0.0", "space_id": space, "mode": apppackages.Objects}
	plan := call("lastapp_package_read", registry.Row{"action": "plan", "selection": sel})
	sel["plan_hash"] = plan["plan_hash"]
	stage := call("lastapp_package", registry.Row{"action": "stage", "plan": sel, "request_key": registry.NewID()})
	id := stage["workspace_id"]
	wait := func(state string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			row := call("lastapp_package_read", registry.Row{"action": "status", "workspace_id": id})
			if row["state"] == state {
				return
			}
			if row["state"] == "failed" {
				t.Fatal(row)
			}
			time.Sleep(300 * time.Millisecond)
		}
		t.Fatal("agent stage did not reach", state)
	}
	wait("ready")
	call("lastapp_package", registry.Row{"action": "activate", "workspace_id": id, "request_key": registry.NewID()})
	wait("active")
}
