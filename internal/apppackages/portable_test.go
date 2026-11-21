package apppackages

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entity"
)

func synthetic(key string) entity.Definition {
	return entity.Definition{
		Format: 1, Key: key, Label: key,
		Fields: []entity.Field{{Key: "name", Label: "Name", Type: "text", MaxLength: 96}},
	}
}

func TestAttachCreateContract(t *testing.T) {
	original := synthetic("contact")
	original.Fields = append(original.Fields, entity.Field{Key: "note", Label: "Note", Type: "text", MaxLength: 96})
	original.Permissions = []entity.Permission{{Key: "create", Operation: "create", Fields: []string{"name"}}}
	d := delivery.ConfiguredEntity{ID: "contact", Definition: original}
	permission := entity.PermissionID(d.ID, "create")
	gates := map[string][]entity.Gate{permission: {{Permission: permission, Fields: []string{"name"}}}}
	customized := original
	customized.Fields = append([]entity.Field(nil), original.Fields...)
	if e := compatibleEntity(d, customized, gates); e != nil {
		t.Fatal(e)
	}
	customized.Fields[1].Required = true
	if e := compatibleEntity(d, customized, gates); e == nil {
		t.Fatal("attach permitted a required field outside the App's create contract")
	}
	customized.Fields[1].Default = &entity.Expr{Literal: "Default note"}
	if e := compatibleEntity(d, customized, gates); e != nil {
		t.Fatal("default supplies the required field", e)
	}
}

func TestPortableCyclesAndIDs(t *testing.T) {
	left, right := synthetic("left"), synthetic("right")
	left.Fields = append(left.Fields, entity.Field{Key: "neighbor", Label: "Neighbor", Type: "reference", Target: "right"})
	right.Fields = append(right.Fields, entity.Field{Key: "neighbor", Label: "Neighbor", Type: "reference", Target: "left"})
	left.Permissions = []entity.Permission{{Key: "view", Label: "View", Operation: "read", Roles: []string{"observer"}, Fields: []string{"name"}}}
	m := Manifest{
		Format: 1, Key: "portable", Version: "1.0.0", Name: "Portable package", Definitions: []entity.Definition{left, right},
		Roles: []Role{{Key: "observer", Name: "Observer"}},
		App: &App{
			Key: "portable", Name: "Portable",
			Access:    entity.AppAccess{Ceiling: []entity.Gate{{Permission: "left/view", Fields: []string{"name"}}}},
			Microapps: []Microapp{{Key: "overview", Name: "Overview", Version: "1.0.0"}},
		},
	}
	if e := m.validate(); e != nil {
		t.Fatal(e)
	}
	a, e := m.configuration("first", true, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	b, e := m.configuration("second", true, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	if a.Entities[0].ID == b.Entities[0].ID || a.Apps[0].ID == b.Apps[0].ID {
		t.Fatal("identities were reused across spaces")
	}
	for i, v := range a.Entities {
		if v.Definition.Fields[1].Target != a.Entities[1-i].ID {
			t.Fatal("cycle did not map")
		}
	}
	if a.Entities[0].Definition.Permissions[0].Roles[0] != a.Roles[0].ID || a.Apps[0].Access.Ceiling[0].Permission != entity.PermissionID(a.Entities[0].ID, "view") {
		t.Fatal("permission dependencies did not map")
	}
	if m.Definitions[0].Fields[1].Target != "right" || m.Definitions[0].Permissions[0].Roles[0] != "observer" || m.App.Access.Ceiling[0].Permission != "left/view" {
		t.Fatal("binding modified the source manifest")
	}
}

func fixtureAtLimit() Manifest {
	m := Manifest{Format: Format, Key: "bounds", Version: "1.0.0", Name: "Bounds"}
	for i := range MaxItems {
		m.Definitions = append(m.Definitions, synthetic(fmt.Sprintf("entry%d", i)))
	}
	return m
}

func TestManifestBounds(t *testing.T) {
	m := fixtureAtLimit()
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = decodeManifest(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	m.Definitions = append(m.Definitions, synthetic("extra"))
	if e = m.validate(); e == nil {
		t.Fatal("too many package items accepted")
	}
	if _, e = decodeManifest(strings.NewReader(strings.Repeat(" ", MaxManifest+1))); e == nil {
		t.Fatal("oversized manifest accepted")
	}
}

func BenchmarkDecodeManifestAtItemLimit(b *testing.B) {
	m := fixtureAtLimit()
	data, e := json.Marshal(m)
	if e != nil {
		b.Fatal(e)
	}
	raw := string(data)
	b.ReportAllocs()
	for b.Loop() {
		if _, e = decodeManifest(strings.NewReader(raw)); e != nil {
			b.Fatal(e)
		}
	}
}
