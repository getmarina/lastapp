package apppackages

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entities"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

const (
	Objects = "objects"
	Full    = "package"
	Attach  = "attach"
)

type Selection struct {
	Key     string `json:"key"`
	Version string `json:"version"`
	Space   string `json:"space_id"`
	Mode    string `json:"mode"`
}

type Plan struct {
	Quota    QuotaUse `json:"quota_use"`
	Audience []string `json:"launch_roles,omitempty"`
	Selection
	Digest         string                      `json:"plan_hash"`
	Base           string                      `json:"base_hash"`
	PackageDigest  string                      `json:"package_digest"`
	Changes        delivery.SpaceConfiguration `json:"changes"`
	InheritingApps []string                    `json:"inheriting_apps"`
	Dependencies   map[string]string           `json:"dependencies"`
}

type QuotaUse struct {
	Workspaces int `json:"workspaces"`
	Entities   int `json:"entities"`
	Roles      int `json:"roles"`
	Apps       int `json:"apps"`
	Microapps  int `json:"microapps"`
}

type StageInput struct {
	Selection
	Digest     string `json:"plan_hash"`
	RequestKey string `json:"request_key,omitempty"`
}

type Stage struct {
	FailedStep   *string `json:"failed_step,omitempty"`
	Attempts     int64   `json:"attempts"`
	ID           string  `json:"id"`
	HasApp       bool    `json:"has_app"`
	PreviewApp   *string `json:"-"`
	Installation string  `json:"installation_id"`
	Workspace    string  `json:"workspace_id"`
	Key          string  `json:"key"`
	Version      string  `json:"version"`
	Mode         string  `json:"mode"`
	State        string  `json:"state"`
	Release      *string `json:"release_id"`
	Operation    *string `json:"operation_id"`
	Error        *string `json:"error_message"`
	Revision     int64   `json:"revision"`
	Space        string  `json:"space_id"`
	Preview      string  `json:"preview_url,omitempty"`
	App          *string `json:"app_id"`
	Owner        string  `json:"-"`
	Grant        *string `json:"-"`
}

const stageSQL = `SELECT i.id AS installation_id,x.workspace_id,i.package_key,i.version,x.mode,COALESCE(d.state,w.state) AS stage_state,x.release_id,x.deployment_id,
	COALESCE(d.error_message,w.error_message) AS error_message,COALESCE(d.revision,w.revision) AS stage_revision,i.space_id,d.target_app_id,w.owner_user_id,w.owner_agent_grant_id,x.workspace_id AS id,cv.build_json IS NOT NULL AS has_app,rel.app_id AS preview_app,COALESCE(d.failed_state,w.preparation_step) AS failed_step,COALESCE(d.attempts,0) AS attempts
	FROM package_stages x JOIN package_installations i ON i.id=x.installation_id
	JOIN development_workspaces w ON w.id=x.workspace_id LEFT JOIN release_deployments d ON d.id=x.deployment_id
	JOIN catalog_packages cv ON cv.package_key=i.package_key AND cv.version=i.version
	LEFT JOIN app_releases rel ON rel.id=x.release_id`

func (v *Stage) ScanFields() []any {
	return []any{&v.Installation, &v.Workspace, &v.Key, &v.Version, &v.Mode, &v.State, &v.Release, &v.Operation, &v.Error, &v.Revision, &v.Space, &v.App, &v.Owner, &v.Grant, &v.ID, &v.HasApp, &v.PreviewApp, &v.FailedStep, &v.Attempts}
}

func (s *Service) scoped(ctx context.Context, q registry.Queryer, actor op.Actor, tenant, space, action string) error {
	if registry.ID(tenant) != nil || registry.ID(space) != nil {
		return registry.Missing()
	}
	return s.Run.Check(ctx, q, actor, action, access.Resource{Tenant: tenant, Space: space}, false)
}

func (s *Service) Plan(ctx context.Context, actor op.Actor, tenant string, in Selection) (Plan, error) {
	if e := s.scoped(ctx, s.DB, actor, tenant, in.Space, "workspace.manage"); e != nil {
		return Plan{}, e
	}
	if e := s.productionSpace(ctx, s.DB, tenant, in.Space); e != nil {
		return Plan{}, e
	}
	if e := delivery.CheckWorkspaceQuota(ctx, s.DB, in.Space, actor.Principal.ID); e != nil {
		return Plan{}, e
	}
	if e := s.Delivery.CheckPackagePreviewStorage(ctx, s.DB, access.Resource{Tenant: tenant, Space: in.Space}); e != nil {
		return Plan{}, e
	}
	base, e := s.Delivery.ProductionConfiguration(ctx, s.DB, in.Space)
	if e != nil {
		return Plan{}, e
	}
	return s.plan(ctx, s.DB, tenant, in, base)
}

func (s *Service) productionSpace(ctx context.Context, q registry.Queryer, tenant, space string) error {
	var id string
	return registry.Lookup(ctx, q, "SELECT id FROM spaces WHERE tenant_id=? AND id=? AND state='active' AND workspace_id IS NULL", []any{tenant, space}, &id)
}

func (s *Service) plan(ctx context.Context, q registry.Queryer, tenant string, in Selection, base delivery.SpaceConfiguration) (Plan, error) {
	p := Plan{Selection: in, Base: entity.Hash(base), InheritingApps: []string{}}
	if in.Mode != Objects && in.Mode != Full && in.Mode != Attach {
		return p, registry.Bad("Choose objects, package, or attach.")
	}
	v, e := s.lookup(ctx, q, in.Key, in.Version)
	if e != nil {
		return p, e
	}
	if v.Retired && in.Mode != Attach {
		return p, registry.Conflict("This package version is retired. Choose another version.")
	}
	if in.Mode != Objects && (v.Manifest.App == nil || v.Build == nil || v.Source == nil || s.Delivery.HostedOrigin(in.Space) == "") {
		return p, registry.Conflict("This package has no installable App or App hosting is unavailable.")
	}
	var installation, previous string
	e = q.QueryRowContext(ctx, "SELECT id,version FROM package_installations WHERE space_id=? AND package_key=?", in.Space, in.Key).Scan(&installation, &previous)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return p, e
	}
	if in.Mode == Attach {
		if previous != in.Version {
			return p, registry.Conflict("Attach requires the installed objects version.")
		}
		var state string
		e = registry.Lookup(ctx, q, "SELECT d.state FROM package_stages x JOIN release_deployments d ON d.id=x.deployment_id WHERE x.installation_id=? AND x.mode='objects'", []any{installation}, &state)
		if e != nil || state != "active" {
			return p, registry.Conflict("Install the business objects before attaching the App.")
		}
	} else if installation != "" {
		return p, registry.Conflict("This Space already has a staged or installed copy of the package.")
	}
	actual := map[string]delivery.ConfiguredEntity{}
	for _, d := range base.Entities {
		actual[d.ID] = d
	}
	p.Dependencies = v.Manifest.Dependencies
	dependency := func(pkg, key string) (string, error) {
		id := itemID(in.Space+":"+pkg, "entity", key)
		if actual[id].State != registry.StateActive {
			return "", registry.Conflict("Missing compatible prerequisite entity: " + pkg + "/" + key)
		}
		return id, nil
	}
	for pkg, version := range v.Manifest.Dependencies {
		var found int
		e = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM package_installations i JOIN package_stages x ON x.installation_id=i.id
			JOIN release_deployments d ON d.id=x.deployment_id AND d.state='active' WHERE i.space_id=? AND i.package_key=? AND i.version=? AND x.mode<>'attach'`, in.Space, pkg, version).Scan(&found)
		if e != nil {
			return p, e
		}
		if found != 1 {
			return p, registry.Conflict("Install prerequisite " + pkg + " " + version + " in this Space first.")
		}
	}
	scope := in.Space + ":" + in.Key
	p.Changes, e = v.Manifest.configuration(scope, in.Mode != Objects, in.Mode == Attach, dependency)
	if in.Mode != Objects {
		keys := v.Manifest.App.LaunchRoles
		if len(keys) == 0 {
			for _, r := range v.Manifest.Roles {
				keys = append(keys, r.Key)
			}
			for _, r := range v.Manifest.App.Roles {
				keys = append(keys, "app/"+r.Key)
			}
		}
		for _, key := range keys {
			p.Audience = append(p.Audience, itemID(scope, "role", key))
		}
		slices.Sort(p.Audience)
		p.Audience = slices.Compact(p.Audience)
	}
	if e != nil {
		return p, e
	}
	if in.Mode == Attach {
		expected, err := v.Manifest.configuration(scope, true, false, dependency)
		if err != nil {
			return p, err
		}
		if err = compatible(expected, base); err != nil {
			return p, err
		}
	}
	if e = conflicts(ctx, q, in.Space, p.Changes); e != nil {
		return p, e
	}
	if e = entities.CheckSpaceCapacity(ctx, q, in.Space, len(p.Changes.Entities)); e != nil {
		return p, e
	}
	if e = s.Delivery.ConfigurationQuota(ctx, q, tenant, p.Changes); e != nil {
		return p, e
	}
	if len(p.Changes.Entities) > 0 {
		for _, a := range base.Apps {
			if a.State == registry.StateActive && a.Access.Inherit {
				p.InheritingApps = append(p.InheritingApps, a.Name)
			}
		}
	}
	p.Quota = QuotaUse{Workspaces: 1, Entities: len(p.Changes.Entities), Roles: len(p.Changes.Roles), Apps: len(p.Changes.Apps), Microapps: len(p.Changes.Microapps)}
	p.PackageDigest = v.Digest
	p.Digest = entity.Hash(p)
	return p, nil
}

func conflicts(ctx context.Context, q registry.Queryer, space string, c delivery.SpaceConfiguration) error {
	entities, roles, apps := []string{}, []string{}, []string{}
	for _, d := range c.Entities {
		entities = append(entities, d.Definition.Key)
	}
	for _, r := range c.Roles {
		if r.App == "" {
			roles = append(roles, r.Code)
		}
	}
	for _, a := range c.Apps {
		apps = append(apps, a.Slug)
	}
	for _, check := range []struct {
		base, parent, label string
		keys                []string
	}{
		{"SELECT entity_key FROM entity_definitions WHERE space_id=? AND entity_key IN (", space, "Entity", entities},
		{"SELECT code FROM roles WHERE owner_key=? AND code IN (", "s:" + space, "Space role", roles},
		{"SELECT slug FROM apps WHERE space_id=? AND slug IN (", space, "App", apps},
	} {
		if len(check.keys) == 0 {
			continue
		}
		params := []any{check.parent}
		for _, k := range check.keys {
			params = append(params, k)
		}
		var key string
		e := q.QueryRowContext(ctx, check.base+strings.Repeat("?,", len(check.keys)-1)+"?) LIMIT 1", params...).Scan(&key)
		switch {
		case e == nil:
			return registry.Conflict(check.label + " already exists: " + key + ".")
		case !errors.Is(e, sql.ErrNoRows):
			return e
		}
	}
	return nil
}

func compatible(expected, current delivery.SpaceConfiguration) error {
	active := map[string]entity.Definition{}
	for _, d := range current.Entities {
		if d.State == registry.StateActive {
			active[d.ID] = d.Definition
		}
	}
	roleExists := map[string]bool{}
	for _, r := range current.Roles {
		roleExists[r.ID] = r.State == registry.StateActive
	}
	for _, r := range expected.Roles {
		if r.App == "" && !roleExists[r.ID] {
			return registry.Conflict("Required Space role is unavailable: " + r.Code)
		}
	}
	gates := map[string][]entity.Gate{}
	for _, app := range expected.Apps {
		for _, group := range [][]entity.Gate{app.Access.Ceiling, app.Access.Restrictions} {
			for _, g := range group {
				gates[g.Permission] = append(gates[g.Permission], g)
			}
		}
	}
	for _, d := range expected.Entities {
		got, ok := active[d.ID]
		if !ok {
			return registry.Conflict("Required entity is unavailable: " + d.Definition.Key)
		}
		if e := compatibleEntity(d, got, gates); e != nil {
			return e
		}
	}
	return nil
}

func compatibleEntity(d delivery.ConfiguredEntity, got entity.Definition, gates map[string][]entity.Gate) error {
	needed := map[string]bool{}
	creates := false
	for _, p := range d.Definition.Permissions {
		used := gates[entity.PermissionID(d.ID, p.Key)]
		if len(used) == 0 {
			continue
		}
		if !slices.ContainsFunc(got.Permissions, func(other entity.Permission) bool {
			other.Label = p.Label
			return entity.Hash(p) == entity.Hash(other)
		}) {
			return registry.Conflict("A required permission path changed: " + d.Definition.Key + "/" + p.Key + ".")
		}
		for _, g := range used {
			for _, f := range g.Fields {
				needed[f] = true
			}
			if g.When != nil && got.CheckCondition(*g.When, false) != nil {
				return registry.Conflict("An App access condition no longer fits " + d.Definition.Key + ".")
			}
		}
		creates = creates || p.Operation == "create"
	}
	for key := range needed {
		f, known := d.Definition.Field(key)
		if !known {
			continue
		} // Built-in record fields have a fixed contract.
		other, ok := got.Field(f.Key)
		f.Label, other.Label = "", ""
		f.ValueLabels, other.ValueLabels = nil, nil
		if !ok || entity.Hash(f) != entity.Hash(other) {
			return registry.Conflict("Client contract changed for " + d.Definition.Key + "." + f.Key + ".")
		}
	}
	if creates {
		for _, f := range got.Fields {
			original, existed := d.Definition.Field(f.Key)
			alreadyRequired := existed && original.Required && original.Default == nil && original.Calculation == nil
			if f.Required && f.Default == nil && f.Calculation == nil && !alreadyRequired {
				return registry.Conflict("Client cannot supply required field " + d.Definition.Key + "." + f.Key + ".")
			}
		}
	}
	return nil
}

func (s *Service) Stage(ctx context.Context, actor op.Actor, tenant string, in StageInput) (Stage, error) {
	if e := s.scoped(ctx, s.DB, actor, tenant, in.Space, "workspace.manage"); e != nil {
		return Stage{}, e
	}
	id := actor.OperationID()
	if actor.Agent == nil {
		if registry.ID(in.RequestKey) != nil {
			return Stage{}, registry.Bad("A UUID request_key is required.")
		}
		id = itemID(tenant+":"+actor.Principal.ID, "request", in.RequestKey)
	}
	existing, e := s.stagedRequest(ctx, actor, tenant, id, in)
	if e == nil {
		return existing, nil
	}
	if op.Classify(e).Status != 404 {
		return Stage{}, e
	}
	_, e = s.Delivery.StageWorkspace(ctx, actor, in.Space, delivery.WorkspaceCreateInput{ID: id, Name: "Package " + in.Key}, func(ctx context.Context, tx *sql.Tx, base delivery.SpaceConfiguration) (delivery.SpaceConfiguration, error) {
		if e := s.productionSpace(ctx, tx, tenant, in.Space); e != nil {
			return delivery.SpaceConfiguration{}, e
		}
		p, e := s.plan(ctx, tx, tenant, in.Selection, base)
		if e != nil {
			return p.Changes, e
		}
		if p.Digest != in.Digest {
			return p.Changes, registry.Conflict("The reviewed plan changed. Plan the installation again.")
		}
		installation := itemID(in.Space, "installation", in.Key)
		if in.Mode != Attach {
			if _, e = tx.ExecContext(ctx, "INSERT INTO package_installations(id,tenant_id,space_id,package_key,version) VALUES(?,?,?,?,?)", installation, tenant, in.Space, in.Key, in.Version); e != nil {
				return p.Changes, e
			}
		}
		b, _ := json.Marshal(p)
		_, e = tx.ExecContext(ctx, "INSERT INTO package_stages(workspace_id,installation_id,mode,plan_hash,plan_json) VALUES(?,?,?,?,?)", id, installation, in.Mode, p.Digest, b)
		return p.Changes, e
	})
	if e != nil {
		// Another request with the same key may already have committed.
		if got, err := s.stagedRequest(ctx, actor, tenant, id, in); err == nil {
			return got, nil
		}
		return Stage{}, e
	}
	return s.Status(ctx, actor, tenant, id)
}

func (s *Service) Status(ctx context.Context, actor op.Actor, tenant, id string) (Stage, error) {
	v, e := registry.ScanOne[Stage](ctx, s.DB, stageSQL+" WHERE i.tenant_id=? AND x.workspace_id=?", tenant, id)
	if e != nil {
		return v, e
	}
	if e = s.scoped(ctx, s.DB, actor, tenant, v.Space, "workspace.manage"); e != nil {
		return Stage{}, e
	}
	if v.Operation == nil && (v.Owner != actor.Principal.ID || access.Deref(v.Grant) != grantID(actor)) {
		return Stage{}, registry.Missing()
	}
	s.addPreview(actor, &v)
	return v, nil
}

func grantID(a op.Actor) string {
	if a.Principal.Agent != nil {
		return a.Principal.Agent.ID
	}
	return ""
}

func (s *Service) addPreview(actor op.Actor, v *Stage) {
	if v.PreviewApp != nil && v.Owner == actor.Principal.ID && access.Deref(v.Grant) == grantID(actor) {
		v.Preview = s.Delivery.HostedOrigin(*v.PreviewApp) + "/"
	}
}

func (s *Service) stagedRequest(ctx context.Context, actor op.Actor, tenant, id string, in StageInput) (Stage, error) {
	v, e := s.Status(ctx, actor, tenant, id)
	if e != nil {
		return v, e
	}
	var digest string
	if e = registry.Lookup(ctx, s.DB, "SELECT plan_hash FROM package_stages WHERE workspace_id=?", []any{id}, &digest); e != nil {
		return v, e
	}
	if digest != in.Digest || v.Space != in.Space || v.Key != in.Key || v.Version != in.Version || v.Mode != in.Mode {
		return v, registry.Conflict("Request ID already stages different content.")
	}
	return v, nil
}
