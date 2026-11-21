package delivery

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/inventory"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

func (s *Service) ReleaseSource(ctx context.Context, actor op.Actor, release string) (*os.File, error) {
	if _, e := s.releaseRow(ctx, s.DB, actor.Principal, release); e != nil {
		return nil, e
	}
	var hash string
	if e := registry.Lookup(ctx, s.DB, "SELECT content_hash FROM release_sources WHERE release_id=?", []any{release}, &hash); e != nil {
		return nil, e
	}
	f, e := artifact.OpenSource(s.ArtifactDirectory(), hash)
	if errors.Is(e, os.ErrNotExist) {
		return nil, registry.Missing()
	}
	return f, e
}

// SourceForApp finds the selected source for an App or its workspace copy.
// Choosing a build without source never falls back to an older release.
func (s *Service) SourceForApp(ctx context.Context, actor op.Actor, app string) (string, error) {
	a, e := inventory.AppRow(ctx, s.DB, app)
	if e != nil {
		return "", e
	}
	if e = s.Run.Authorize(ctx, actor, "release.manage", access.Resource{Tenant: a.TenantID, Space: a.SpaceID, App: a.ID}); e != nil {
		return "", e
	}
	id := access.Deref(a.ActiveRelease)
	if id == "" {
		e = s.DB.QueryRowContext(ctx, `SELECT COALESCE(production.active_release_id,'') FROM workspace_items w
		 JOIN development_workspaces ws ON ws.id=w.workspace_id AND ws.backing_space_id=?
		 JOIN apps production ON production.id=w.production_id
		 WHERE w.kind='app' AND w.development_id=?`, a.SpaceID, a.ID).Scan(&id)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return "", e
		}
	}
	if id == "" {
		return "", nil
	}
	return s.SourceReference(ctx, actor, id)
}

func (s *Service) SourceReference(ctx context.Context, actor op.Actor, release string) (string, error) {
	if _, e := s.releaseRow(ctx, s.DB, actor.Principal, release); e != nil {
		return "", e
	}
	var exists int
	e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM release_sources WHERE release_id=?", release).Scan(&exists)
	if exists == 0 {
		return "", e
	}
	return release, e
}

func (s *Service) WorkspaceConfiguration(ctx context.Context, actor op.Actor, app string) (registry.Row, error) {
	a, _, e := s.ownedDevelopmentApp(ctx, s.DB, actor.Principal, app)
	if e != nil {
		return nil, e
	}
	return s.localConfiguration(a.ID), nil
}

func (s *Service) localConfiguration(app string) registry.Row {
	return registry.Row{"apiOrigin": s.Config.PublicURL, "appID": app, "environment": "development"}
}
