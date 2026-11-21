package apppackages

import (
	"context"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/op"
)

func (v *Version) ScanFields() []any {
	return []any{&v.Key, &v.Version, &v.Digest, &v.Name, &v.HasApp, &v.ID}
}

func (s *Service) member(ctx context.Context, actor op.Actor, tenant string) error {
	if actor.Principal.Agent != nil {
		if e := s.Run.Access.CheckAgent(ctx, s.DB, actor.Principal, access.Resource{Tenant: tenant}, false); e != nil {
			return e
		}
	}
	return s.Run.Access.Member(ctx, s.DB, actor.Principal, tenant, true)
}

func (s *Service) Catalog(ctx context.Context, actor op.Actor, tenant string, page op.PageRequest) (op.Page[Version], error) {
	if e := s.member(ctx, actor, tenant); e != nil {
		return op.Page[Version]{}, e
	}
	return op.PageScan[Version](ctx, s.DB, page, op.CollectionFor("package-catalog"), `SELECT p.package_key,p.version,p.digest,
		JSON_UNQUOTE(JSON_EXTRACT(p.manifest_json,'$.name')) AS display_name,p.build_json IS NOT NULL AS has_app,CONCAT(p.package_key,'@',p.version) AS id
		FROM catalog_packages p WHERE p.retired_at IS NULL ORDER BY p.package_key,p.published_at DESC,p.version`)
}

func (s *Service) Inspect(ctx context.Context, actor op.Actor, tenant, key, version string) (Manifest, error) {
	if e := s.member(ctx, actor, tenant); e != nil {
		return Manifest{}, e
	}
	v, e := s.lookup(ctx, s.DB, key, version)
	return v.Manifest, e
}

func (s *Service) Stages(ctx context.Context, actor op.Actor, tenant string, page op.PageRequest) (op.Page[Stage], error) {
	sc, e := s.Run.Scopes(ctx, actor, tenant, "workspace.manage")
	if e != nil {
		return op.Page[Stage]{}, e
	}
	filter, args := access.ScopeSQL(sc, "i.space_id", "''")
	args = append([]any{tenant, actor.Principal.ID, grantID(actor)}, args...)
	result, e := op.PageScan[Stage](ctx, s.DB, page, op.CollectionFor("package-stages"), stageSQL+` WHERE i.tenant_id=?
		AND (x.deployment_id IS NOT NULL OR (w.owner_user_id=? AND COALESCE(w.owner_agent_grant_id,'')=?))
		AND `+filter+" ORDER BY w.created_at DESC,x.workspace_id", args...)
	for i := range result.Items {
		s.addPreview(actor, &result.Items[i])
	}
	return result, e
}

type LaunchApp struct {
	ID   string `json:"id"`
	Name string `json:"display_name"`
	URL  string `json:"url"`
}

func (v *LaunchApp) ScanFields() []any { return []any{&v.ID, &v.Name, &v.URL} }

func (s *Service) MyApps(ctx context.Context, actor op.Actor, tenant string, page op.PageRequest) (op.Page[LaunchApp], error) {
	if e := s.member(ctx, actor, tenant); e != nil {
		return op.Page[LaunchApp]{}, e
	}
	return op.PageScan[LaunchApp](ctx, s.DB, page, op.CollectionFor("my-apps"), `SELECT DISTINCT a.id,a.display_name,a.approved_origin
	 FROM package_installations i JOIN package_stages x ON x.installation_id=i.id
	 JOIN release_deployments d ON d.id=x.deployment_id AND d.state='active'
	 JOIN apps a ON a.id=d.target_app_id AND a.state='active' AND a.active_release_id IS NOT NULL AND a.approved_origin IS NOT NULL
	 JOIN spaces sp ON sp.id=i.space_id AND sp.state='active'
	 JOIN ( `+access.EffectiveAssignmentSQL+` ) ar ON ar.domain='data' AND ((ar.app_id IS NULL AND ar.space_id=i.space_id) OR ar.app_id=a.id)
	 JOIN package_app_audience audience ON audience.app_id=a.id AND audience.role_id=ar.role_id
 WHERE i.tenant_id=? ORDER BY a.display_name,a.id`, actor.Principal.ID, tenant, tenant)
}
