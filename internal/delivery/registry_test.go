package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mariadb "github.com/aantich/go-mariadb"
	"github.com/pressly/goose/v3"

	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/testdb"
)

// registryFixture creates a fresh registry database from the ordered
// migrations on the disposable test server (internal/testdb, its
// administrator account) and returns one pinned connection with
// foreign key checks disabled, so a test inserts only the rows its query
// reads. The test owns the database and drops it on cleanup.
func registryFixture(t *testing.T) *sql.Conn {
	t.Helper()
	conn, _ := registryAt(t, 0)
	return conn
}

// registryAt is registryFixture migrated up to version (0: all), with the
// database handle for further migrations or concurrent transactions.
func registryAt(t *testing.T, version int64) (*sql.Conn, *sql.DB) {
	t.Helper()
	cfg := testdb.Load(t, testdb.ConfigEnv).AdminDSN(t)
	cfg.Loc = time.UTC
	connector, e := mariadb.NewConnector(cfg)
	if e != nil {
		t.Fatal(e)
	}
	admin := sql.OpenDB(connector)
	name := "delivery_test_" + strings.ReplaceAll(registry.NewID(), "-", "")
	if _, e = admin.Exec("CREATE DATABASE `" + name + "`"); e != nil {
		admin.Close()
		t.Fatal(e)
	}
	cfg = cfg.Clone()
	cfg.DBName = name
	if connector, e = mariadb.NewConnector(cfg); e != nil {
		t.Fatal(e)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		db.Close()
		if _, e := admin.Exec("DROP DATABASE `" + name + "`"); e != nil {
			t.Error("fixture database cleanup failed", e)
		}
		admin.Close()
	})
	goose.SetBaseFS(nil)
	goose.SetLogger(goose.NopLogger())
	if e = goose.SetDialect("mysql"); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if version == 0 {
		e = goose.UpContext(ctx, db, migrationsDir)
	} else {
		e = goose.UpToContext(ctx, db, migrationsDir, version)
	}
	if e != nil {
		t.Fatal(e)
	}
	conn, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { conn.Close() })
	if _, e = conn.ExecContext(ctx, "SET SESSION foreign_key_checks=0"); e != nil {
		t.Fatal(e)
	}
	return conn, db
}

var migrationsDir = filepath.Join("..", "control", "migrations")

func exec(t *testing.T, q registry.Queryer, query string, args ...any) {
	t.Helper()
	if _, e := q.ExecContext(context.Background(), query, args...); e != nil {
		t.Fatal(e)
	}
}

func apiStatus(e error) int {
	var ae *registry.APIError
	if errors.As(e, &ae) {
		return ae.Status
	}
	return 0
}

// A deployment's captured entities resolve references only while it prepares
// or publishes storage, and only its active entities are candidates. The
// publisher's authority recheck applies only to a publishing deployment.
func TestDeploymentEntitiesFollowState(t *testing.T) {
	q := registryFixture(t)
	ctx := context.Background()
	snap := captured()
	archived := snap.Entities[0]
	archived.ID, archived.State = registry.NewID(), registry.StateArchived
	snap.Entities = append(snap.Entities, archived)
	plan, _ := json.Marshal(deploymentPlan{Snapshot: snap, Versions: map[string]string{}})
	ids := map[string]string{}
	for _, state := range []string{"queued", "preparing", "publishing", "active", "failed"} {
		ids[state] = registry.NewID()
		exec(t, q, "INSERT INTO release_deployments(id,tenant_id,space_id,release_id,target_app_id,created_by,plan_json,state) VALUES(?,?,?,?,?,?,?,?)", ids[state], registry.NewID(), registry.NewID(), registry.NewID(), registry.NewID(), registry.NewID(), plan, state)
	}
	for state, id := range ids {
		got, e := DeploymentEntities(ctx, q, id)
		if state == "preparing" || state == "publishing" {
			if e != nil || len(got) != 1 || !got[prodEntity] {
				t.Fatalf("%s: entities %v, %v", state, got, e)
			}
			continue
		}
		if apiStatus(e) != 404 {
			t.Fatalf("%s deployment resolved candidate entities: %v %v", state, got, e)
		}
	}
	s := &Service{}
	for _, state := range []string{"queued", "preparing", "active", "failed"} {
		if e := s.PublishingDeploymentAuthority(ctx, q, ids[state]); apiStatus(e) != 404 {
			t.Fatalf("%s deployment passed as publishing: %v", state, e)
		}
	}
}

// The production App quota counts production Apps only and the net effect of
// the captured snapshot (new Apps add, archived existing Apps subtract).
func TestSnapshotAppQuota(t *testing.T) {
	q := registryFixture(t)
	ctx := context.Background()
	plan, tenant, prod, dev := registry.NewID(), registry.NewID(), registry.NewID(), registry.NewID()
	exec(t, q, "INSERT INTO plans(id,code,display_name,allow_shared_database,allow_dedicated_database,allow_managed_database,allow_registered_database,default_layout,max_apps) VALUES(?,?,?,1,1,1,1,'shared',2)", plan, "quota-"+plan[:8], "Quota")
	exec(t, q, "INSERT INTO tenants(id,slug,display_name,plan_id,created_by) VALUES(?,?,?,?,?)", tenant, "t-"+tenant[:8], "Quota", plan, registry.NewID())
	exec(t, q, "INSERT INTO spaces(id,tenant_id,slug,display_name,database_resource_id,database_layout) VALUES(?,?,?,?,?,'shared')", prod, tenant, "prod", "Production", registry.NewID())
	exec(t, q, "INSERT INTO spaces(id,tenant_id,slug,display_name,database_resource_id,database_layout,workspace_id) VALUES(?,?,?,?,?,'shared',?)", dev, tenant, "dev", "Workspace", registry.NewID(), registry.NewID())
	existing := registry.NewID()
	exec(t, q, "INSERT INTO apps(id,tenant_id,space_id,slug,display_name) VALUES(?,?,?,?,?)", existing, tenant, prod, "existing", "Existing")
	for i := range 3 {
		exec(t, q, "INSERT INTO apps(id,tenant_id,space_id,slug,display_name) VALUES(?,?,?,?,?)", registry.NewID(), tenant, dev, "dev-"+string(rune('a'+i)), "Workspace App")
	}
	app := func(id, state string) ConfiguredApp { return ConfiguredApp{ID: id, Slug: "s", Name: "n", State: state} }
	s := &Service{}
	if e := s.snapshotAppQuota(ctx, q, tenant, SpaceConfiguration{Apps: []ConfiguredApp{app(existing, registry.StateActive), app(registry.NewID(), registry.StateActive)}}); e != nil {
		t.Fatalf("workspace Apps counted against the production quota: %v", e)
	}
	if e := s.snapshotAppQuota(ctx, q, tenant, SpaceConfiguration{Apps: []ConfiguredApp{app(registry.NewID(), registry.StateActive), app(registry.NewID(), registry.StateActive)}}); apiStatus(e) != 409 {
		t.Fatalf("quota exceeded without conflict: %v", e)
	}
	if e := s.snapshotAppQuota(ctx, q, tenant, SpaceConfiguration{Apps: []ConfiguredApp{app(existing, registry.StateArchived), app(registry.NewID(), registry.StateActive), app(registry.NewID(), registry.StateActive)}}); e != nil {
		t.Fatalf("archiving an existing App did not free quota: %v", e)
	}
}

// A workspace or release captures published configuration only.
func TestEnsureSnapshotReady(t *testing.T) {
	q := registryFixture(t)
	ctx := context.Background()
	d := entity.Templates()[0]
	draft, _ := json.Marshal(d)
	insert := func(draft []byte, activeHash string, pending bool) string {
		space, id, version := registry.NewID(), registry.NewID(), registry.NewID()
		var pendingVersion any
		if pending {
			pendingVersion = registry.NewID()
		}
		exec(t, q, "INSERT INTO entity_versions(id,tenant_id,space_id,entity_id,version_number,definition_json,definition_hash,created_by,state) VALUES(?,?,?,?,1,?,?,?,'published')", version, registry.NewID(), space, id, draft, activeHash, registry.NewID())
		exec(t, q, "INSERT INTO entity_definitions(id,tenant_id,space_id,entity_key,label,draft_json,active_version_id,pending_version_id) VALUES(?,?,?,?,?,?,?,?)", id, registry.NewID(), space, d.Key, d.Label, draft, version, pendingVersion)
		return space
	}
	s := &Service{}
	if e := s.ensureSnapshotReady(ctx, q, insert(draft, entity.Hash(d), false)); e != nil {
		t.Fatalf("published Space rejected: %v", e)
	}
	if e := s.ensureSnapshotReady(ctx, q, insert(draft, entity.Hash(d), true)); apiStatus(e) != 409 {
		t.Fatalf("pending publication captured: %v", e)
	}
	changed := d
	changed.Label = "Changed"
	if e := s.ensureSnapshotReady(ctx, q, insert(draft, entity.Hash(changed), false)); apiStatus(e) != 409 {
		t.Fatalf("unpublished draft captured: %v", e)
	}
}
