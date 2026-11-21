package apppackages

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/registry"
)

// Item identifiers are stable within a Space and independent from publisher
// and workspace identities. Only the planner produces these UUIDs.
func itemID(scope, kind, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("lastapp:packages:"+scope+":"+kind+":"+key)).String()
}

var capabilities = []string{"entity-v1", "app-sdk-v1", "static-release-v1"}

func (m Manifest) validate() error {
	if len(m.Requires) > len(capabilities) {
		return registry.Bad("Too many package capabilities.")
	}
	for _, required := range m.Requires {
		if !slices.Contains(capabilities, required) {
			return registry.Bad("Unsupported package capability: " + required)
		}
	}
	if m.Format != Format || len(m.Key) > 48 || !validVersion(m.Version) || registry.Named(m.Key, m.Name) != nil || len(m.Description) > 1000 {
		return registry.Bad("Invalid package format, key, version, or display metadata.")
	}
	if len(m.Definitions) == 0 || len(m.Definitions)+len(m.Roles) > MaxItems || len(m.Dependencies) > MaxDependencies {
		return registry.Bad("Package exceeds its entity, role, or dependency limits.")
	}
	for key, version := range m.Dependencies {
		if len(key) > 48 || registry.Named(key, key) != nil || key == m.Key || !validVersion(version) {
			return registry.Bad("Dependencies require a different package key and an exact version.")
		}
	}
	if m.App != nil {
		a := m.App
		if registry.Named(a.Key, a.Name) != nil || a.Access.Inherit || a.Access.Origin != "" ||
			len(a.Microapps) == 0 || len(a.Microapps) > registry.MaxReleaseMicroapps || len(a.Microapps)+len(a.Roles)+len(m.Definitions)+len(m.Roles)+1 > MaxItems {
			return registry.Bad("Packaged Apps require an explicit access ceiling and bounded micro-apps.")
		}
	}
	_, e := m.configuration("catalog", true, false, nil)
	return e
}

// dependencyID resolves an explicitly named entity in a pinned prerequisite.
type dependencyID func(packageKey, entityKey string) (string, error)

func (m Manifest) configuration(scope string, withApp, attach bool, dependency dependencyID) (delivery.SpaceConfiguration, error) {
	var result delivery.SpaceConfiguration
	entities := map[string]string{}
	roles := map[string]string{}
	for _, d := range m.Definitions {
		if entities[d.Key] != "" {
			return result, registry.Bad("Duplicate entity key: " + d.Key)
		}
		entities[d.Key] = itemID(scope, "entity", d.Key)
	}
	addRoles := func(list []Role, scopeKind, app string) error {
		for _, r := range list {
			key := scopeKind + r.Key
			if registry.Named(r.Key, r.Name) != nil || roles[key] != "" {
				return registry.Bad("Duplicate or invalid role key: " + key)
			}
			id := itemID(scope, "role", key)
			roles[key] = id
			if scopeKind != "" || !attach {
				result.Roles = append(result.Roles, delivery.ConfiguredRole{ID: id, App: app, Code: r.Key, Name: r.Name, State: registry.StateActive})
			}
		}
		return nil
	}
	if e := addRoles(m.Roles, "", ""); e != nil {
		return result, e
	}
	permissions := map[string]entity.Permission{}
	definitions := map[string]entity.Definition{}
	for _, d := range m.Definitions {
		// Copies are shallow only until we touch mutable fields.
		fields := append([]entity.Field(nil), d.Fields...)
		paths := append([]entity.Permission(nil), d.Permissions...)
		var broken error
		targetID := func(target string) string {
			id := entities[target]
			if pkg, key, ok := strings.Cut(target, "/"); ok && m.Dependencies[pkg] != "" && entity.ValidKey(key) {
				id = itemID("catalog:"+pkg, "entity", key)
				if dependency != nil && broken == nil {
					id, broken = dependency(pkg, key)
				}
			}
			if id == "" && broken == nil {
				broken = registry.Bad("Unresolved reference in " + d.Key + ": " + target)
			}
			return id
		}
		roleID := func(role string) string {
			id := roles[role]
			if id == "" && broken == nil {
				broken = registry.Bad("Unresolved Space role: " + role)
			}
			return id
		}
		for i, p := range paths {
			if p.Subject != "" || p.Relationship == "user" || p.Relationship == "team" {
				return result, registry.Bad("Concrete permission subjects are not portable.")
			}
			permissions[d.Key+"/"+p.Key] = p
			paths[i].Roles = append([]string(nil), p.Roles...)
		}
		d.Fields, d.Permissions = fields, paths
		d.RewriteReferences(targetID, roleID)
		if broken != nil {
			return result, broken
		}
		definitions[d.Key] = d
		raw, e := json.Marshal(d)
		if e != nil || len(raw) > entity.MaxDefinitionBytes {
			return result, registry.Bad("Package entity exceeds its encoded definition limit: " + d.Key)
		}
		if ds := d.Validate(); len(ds) > 0 {
			return result, registry.Bad(d.Key + "." + ds[0].Path + ": " + ds[0].Message)
		}
		if !attach {
			result.Entities = append(result.Entities, delivery.ConfiguredEntity{ID: entities[d.Key], State: registry.StateActive, Definition: d})
		}
	}
	if !withApp || m.App == nil {
		return result, nil
	}
	a := m.App
	app := itemID(scope, "app", a.Key)
	if e := addRoles(a.Roles, "app/", app); e != nil {
		return result, e
	}
	access := a.Access
	if len(access.Ceiling)+len(access.Restrictions) > 128 {
		return result, registry.Bad("Too many App access gates.")
	}
	if len(a.LaunchRoles) > MaxItems {
		return result, registry.Bad("Too many App launch roles.")
	}
	for _, role := range a.LaunchRoles {
		if roles[role] == "" {
			return result, registry.Bad("Unknown App discovery role: " + role)
		}
	}
	access.Ceiling = cloneGates(access.Ceiling)
	access.Restrictions = cloneGates(access.Restrictions)
	for _, gates := range [][]entity.Gate{access.Ceiling, access.Restrictions} {
		for _, g := range gates {
			key, _, _ := strings.Cut(g.Permission, "/")
			p, found := permissions[g.Permission]
			if !found {
				return result, registry.Bad("Unresolved App permission: " + g.Permission)
			}
			for _, field := range g.Fields {
				if !slices.Contains(p.Fields, field) {
					return result, registry.Bad("App access field is outside the Space permission: " + field)
				}
			}
			if g.When != nil {
				if e := definitions[key].CheckCondition(*g.When, false); e != nil {
					return result, registry.Bad(e.Error())
				}
			}
			for _, r := range g.Roles {
				if roles[r] == "" || !strings.HasPrefix(r, "app/") {
					return result, registry.Bad("App gates require a local App role: " + r)
				}
			}
		}
	}
	access.RewriteGateReferences(func(ref string) string {
		key, path, _ := strings.Cut(ref, "/")
		return entity.PermissionID(entities[key], path)
	}, func(key string) string { return roles[key] })
	result.Apps = append(result.Apps, delivery.ConfiguredApp{ID: app, Slug: a.Key, Name: a.Name, State: registry.StateActive, Access: access})
	seen := map[string]bool{}
	for i, micro := range a.Microapps {
		if seen[micro.Key] || registry.Named(micro.Key, micro.Name) != nil || !validVersion(micro.Version) {
			return result, registry.Bad("Duplicate or invalid micro-app: " + micro.Key)
		}
		seen[micro.Key] = true
		result.Microapps = append(result.Microapps, delivery.ConfiguredMicroapp{ID: itemID(scope, "microapp", micro.Key), App: app, Slug: micro.Key, Name: micro.Name, State: registry.StateActive, Order: int64(i)})
	}
	return result, nil
}

func cloneGates(in []entity.Gate) []entity.Gate {
	out := slices.Clone(in)
	for i := range out {
		out[i].Roles = slices.Clone(out[i].Roles)
	}
	return out
}
