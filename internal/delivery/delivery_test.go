package delivery

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/config"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/registry"
)

const (
	prodEntity   = "00000000-0000-4000-8000-0000000000e1"
	prodTarget   = "00000000-0000-4000-8000-0000000000e2"
	prodRole     = "00000000-0000-4000-8000-0000000000a1"
	prodApp      = "00000000-0000-4000-8000-0000000000b1"
	prodMicroapp = "00000000-0000-4000-8000-0000000000c1"
)

// captured is a small production snapshot with one of each typed reference.
func captured() SpaceConfiguration {
	d := entity.Templates()[0]
	d.Fields = append(d.Fields, entity.Field{Key: "parent", Label: prodTarget, Type: "reference", Target: prodTarget})
	d.Permissions = []entity.Permission{{Key: "read", Label: "Read", Operation: "read", Roles: []string{prodRole}}}
	gate := entity.Gate{Permission: entity.PermissionID(prodEntity, "read"), Roles: []string{prodRole}}
	return SpaceConfiguration{
		Revision:  7,
		Entities:  []ConfiguredEntity{{Revision: 3, ID: prodEntity, State: registry.StateActive, Definition: d}},
		Roles:     []ConfiguredRole{{Revision: 2, ID: prodRole, App: prodApp, Code: "reviewer", Name: prodRole, State: registry.StateActive}},
		Apps:      []ConfiguredApp{{Revision: 4, ID: prodApp, Slug: "deals", Name: "Deals", State: registry.StateActive, Access: entity.AppAccess{Origin: "https://example.test", Ceiling: []entity.Gate{gate}, Restrictions: []entity.Gate{gate}}}},
		Microapps: []ConfiguredMicroapp{{Revision: 5, ID: prodMicroapp, App: prodApp, Slug: "board", Name: "Board", Artifact: prodApp, Order: 1, State: registry.StateActive}},
	}
}

// Only typed identity references are rewritten; labels and artifact
// references that merely look like IDs stay data, and origins never travel.
func TestRemapSnapshotRewritesTypedReferencesOnly(t *testing.T) {
	base := captured()
	ids := map[string]string{}
	for id := range snapshotIDs(base) {
		ids[id] = registry.NewID()
	}
	ids[prodTarget] = registry.NewID()
	out := remapSnapshot(base, ids)
	e, r, a, m := out.Entities[0], out.Roles[0], out.Apps[0], out.Microapps[0]
	if e.ID != ids[prodEntity] || r.ID != ids[prodRole] || r.App != ids[prodApp] || a.ID != ids[prodApp] || m.ID != ids[prodMicroapp] || m.App != ids[prodApp] {
		t.Fatalf("identities were not remapped: %+v", out)
	}
	parent := e.Definition.Fields[len(e.Definition.Fields)-1]
	if parent.Target != ids[prodTarget] || parent.Label != prodTarget {
		t.Fatalf("reference target %q label %q", parent.Target, parent.Label)
	}
	if e.Definition.Permissions[0].Roles[0] != ids[prodRole] {
		t.Fatal("permission role was not remapped")
	}
	permission := entity.PermissionID(ids[prodEntity], "read")
	for _, g := range append(append([]entity.Gate{}, a.Access.Ceiling...), a.Access.Restrictions...) {
		if g.Permission != permission || g.Roles[0] != ids[prodRole] {
			t.Fatalf("App gate not remapped: %+v", g)
		}
	}
	if a.Access.Origin != "" {
		t.Fatal("approved origin copied into the remapped snapshot")
	}
	if r.Name != prodRole || m.Artifact != prodApp {
		t.Fatal("untyped data that looks like an ID was rewritten")
	}
	if base.Entities[0].ID != prodEntity || base.Apps[0].Access.Origin == "" {
		t.Fatal("remap mutated its input")
	}
}

func TestSnapshotIDsKinds(t *testing.T) {
	got := snapshotIDs(captured())
	want := map[string]string{prodEntity: "entity", prodRole: "role", prodApp: "app", prodMicroapp: "microapp"}
	if len(got) != len(want) {
		t.Fatalf("ids %v", got)
	}
	for id, kind := range want {
		if got[id] != kind {
			t.Fatalf("%s is %q, want %q", id, got[id], kind)
		}
	}
}

// The release contract hash ignores revision counters but not configuration.
func TestSnapshotContractHashIgnoresRevisions(t *testing.T) {
	base := captured()
	bumped := captured()
	bumped.Revision++
	bumped.Entities[0].Revision++
	bumped.Roles[0].Revision++
	bumped.Apps[0].Revision++
	bumped.Microapps[0].Revision++
	if snapshotContractHash(base) != snapshotContractHash(bumped) {
		t.Fatal("revision counters changed the contract hash")
	}
	if entity.Hash(base) == entity.Hash(bumped) {
		t.Fatal("fixture revisions did not differ")
	}
	renamed := captured()
	renamed.Apps[0].Name = "Renamed"
	if snapshotContractHash(base) == snapshotContractHash(renamed) {
		t.Fatal("configuration change kept the contract hash")
	}
}

func TestDecodeDeploymentPlan(t *testing.T) {
	plan := deploymentPlan{Snapshot: captured(), BaseHash: "base", Versions: map[string]string{prodEntity: registry.NewID()}}
	b, _ := json.Marshal(plan)
	var stored registry.JSONValue
	if e := stored.Scan(b); e != nil {
		t.Fatal(e)
	}
	got, e := decodeDeployment(stored)
	if e != nil {
		t.Fatal(e)
	}
	if got.BaseHash != plan.BaseHash || got.Versions[prodEntity] != plan.Versions[prodEntity] || snapshotContractHash(got.Snapshot) != snapshotContractHash(plan.Snapshot) {
		t.Fatalf("plan did not round-trip: %+v", got)
	}
}

// Staged (not yet activated) metadata is inserted inactive.
func TestStagedState(t *testing.T) {
	if stagedState(false, registry.StateActive, registry.StateDisabled) != registry.StateDisabled || stagedState(true, registry.StateActive, registry.StateDisabled) != registry.StateActive {
		t.Fatal("staged state")
	}
}

func TestReleasePinsExactVersions(t *testing.T) {
	for _, v := range []string{"1.2.3", "19.2.0-rc.1+build.5", "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"} {
		if !pinnedVersionRE.MatchString(v) {
			t.Errorf("rejected exact version %q", v)
		}
	}
	for _, v := range []string{"^1.2.3", "~1.2.3", "1.2", "latest", ">=1.0.0", "1.2.3 || 2.0.0", "sha256:ABC"} {
		if pinnedVersionRE.MatchString(v) {
			t.Errorf("accepted range or tag %q", v)
		}
	}
	for _, name := range []string{"react", "@types/react", "lodash.merge"} {
		if !pinnedNameRE.MatchString(name) {
			t.Errorf("rejected package name %q", name)
		}
	}
	for _, name := range []string{"", "../escape", "-flag", "name with space"} {
		if pinnedNameRE.MatchString(name) {
			t.Errorf("accepted package name %q", name)
		}
	}
}

func TestStarterVersionsArePinned(t *testing.T) {
	versions, e := starterVersions()
	if e != nil {
		t.Fatal(e)
	}
	for name, version := range versions {
		if !pinnedVersionRE.MatchString(version) {
			t.Errorf("starter dependency %s is not pinned: %q", name, version)
		}
	}
}

func TestPolicyDerivesFromLimits(t *testing.T) {
	p := Policy()
	if p["max_app_builds"] != registry.MaxAppBuilds || p["max_owned_workspaces"] != registry.MaxOwnedWorkspaces || p["max_space_workspaces"] != registry.MaxSpaceWorkspaces || p["max_snapshot_bytes"] != registry.MaxSpaceSnapshotBytes {
		t.Fatalf("policy %v", p)
	}
	if !reflect.DeepEqual(p["upload"], artifact.Limits) {
		t.Fatal("upload limits are not the artifact store's")
	}
}

func TestHostedOrigins(t *testing.T) {
	const app = "00000000-0000-4000-8000-000000000001"
	for _, tc := range []struct {
		c    config.Config
		want string
	}{
		{config.Config{AppOrigin: "https://Apps.Example.test:443"}, "https://" + app + ".apps.example.test"},
		{config.Config{AppOrigin: "http://apps.example.test:80"}, "http://" + app + ".apps.example.test"},
		{config.Config{AppOrigin: "https://apps.example.test:8443"}, "https://" + app + ".apps.example.test:8443"},
		// Local development defaults to localhost subdomains on the API port.
		{config.Config{Development: true, PublicURL: "http://127.0.0.1:8787"}, "http://" + app + ".localhost:8787"},
		{config.Config{PublicURL: "https://admin.example.test"}, ""},
	} {
		if got := HostedOrigin(&tc.c, app); got != tc.want {
			t.Errorf("%+v: origin %q, want %q", tc.c, got, tc.want)
		}
		s := &Service{Config: &tc.c}
		if s.HostedOrigin(app) != tc.want {
			t.Error("service origin differs from the configuration origin")
		}
	}
	if HostNamespace("https://APPS.example.test.:8443") != ".apps.example.test" || HostNamespace("") != "" {
		t.Fatal("host namespace")
	}
}

func TestArtifactDirectory(t *testing.T) {
	for _, tc := range []struct {
		c    config.Config
		want string
	}{
		{config.Config{ArtifactDirectory: "/srv/builds"}, "/srv/builds"},
		{config.Config{Development: true}, filepath.Join(".local", "artifacts")},
		{config.Config{}, ""},
	} {
		if got := (&Service{Config: &tc.c}).ArtifactDirectory(); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.c, got, tc.want)
		}
	}
}
