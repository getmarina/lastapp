package apppackages

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/databases"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

func (s *Service) frozen(ctx context.Context, q registry.Queryer, id string) (Plan, error) {
	var raw []byte
	var p Plan
	var storedHash string
	e := registry.Lookup(ctx, q, "SELECT plan_json,plan_hash FROM package_stages WHERE workspace_id=?", []any{id}, &raw, &storedHash)
	if e != nil {
		return p, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e = d.Decode(&p); e != nil {
		return p, e
	}
	hash := p.Digest
	p.Digest = ""
	if hash != storedHash || entity.Hash(p) != hash {
		return p, registry.Conflict("Stored package plan failed integrity verification.")
	}
	p.Digest = hash
	return p, nil
}

func (s *Service) Preview(ctx context.Context, actor op.Actor, tenant, id string) (Stage, error) {
	v, e := s.Status(ctx, actor, tenant, id)
	if e != nil {
		return v, e
	}
	if v.Mode == Objects {
		return v, registry.Bad("Business objects have no App preview.")
	}
	var asset catalogVersion
	if v.Release == nil {
		asset, e = s.lookup(ctx, s.DB, v.Key, v.Version)
		if e != nil {
			return v, e
		}
		if asset.Build == nil || asset.Source == nil || artifact.Check(s.Delivery.ArtifactDirectory(), *asset.Build) != nil {
			return v, registry.Conflict("Package build is unavailable or corrupt.")
		}
		f, e := artifact.OpenSource(s.Delivery.ArtifactDirectory(), *asset.Source)
		if e != nil {
			return v, registry.Conflict("Package source is unavailable or corrupt.")
		}
		f.Close()
	}
	res := access.Resource{Tenant: tenant, Space: v.Space}
	e = s.Run.Mutate(ctx, actor, "workspace.manage", res, func(tx *sql.Tx) error {
		var existing *string
		if e := registry.Lookup(ctx, tx, "SELECT release_id FROM package_stages WHERE workspace_id=?", []any{id}, &existing); e != nil {
			return e
		}
		if existing != nil {
			return nil
		}
		p, e := s.frozen(ctx, tx, id)
		if e != nil {
			return e
		}
		if e = s.Delivery.CheckPackageWorkspace(ctx, tx, actor, id, p.Base, p.Changes); e != nil {
			return e
		}
		manifest := delivery.ReleaseManifest{Microapps: map[string]string{}, Dependencies: map[string]string{}}
		for _, m := range asset.Manifest.App.Microapps {
			manifest.Microapps[m.Key] = m.Version
		}
		release, _, e := s.Delivery.PackagePreview(ctx, tx, actor, id, p.Changes.Apps[0].ID, asset.Name, *asset.Build, manifest, *asset.Source)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE package_stages SET release_id=? WHERE workspace_id=?", release, id); e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, actor, tenant, "package.preview", "workspace", id, registry.Row{"release_id": release})
	})
	if e != nil {
		return v, e
	}
	return s.Status(ctx, actor, tenant, id)
}

func (s *Service) Activate(ctx context.Context, actor op.Actor, tenant, id string) (Stage, error) {
	v, e := s.Status(ctx, actor, tenant, id)
	if e != nil {
		return v, e
	}
	res := access.Resource{Tenant: tenant, Space: v.Space}
	if e = s.Run.Authorize(ctx, actor, "production.deploy", res); e != nil {
		return v, e
	}
	if actor.Principal.Agent != nil {
		if e = access.RequireAgent(actor.Principal.Agent, "production.deploy"); e != nil {
			return v, e
		}
	}
	if v.Mode != Objects && v.Release == nil {
		return v, registry.Conflict("Prepare the App preview before activation.")
	}
	if v.Operation == nil && v.Release != nil {
		if e = s.Delivery.EnsureReleaseBuild(ctx, s.DB, *v.Release); e != nil {
			return v, e
		}
	}
	// Resolve already queued activations under the Tenant lock so they can
	// record a receipt without passing the guard for a new production write.
	e = s.Run.Mutate(ctx, actor, "workspace.manage", res, func(tx *sql.Tx) error {
		if e := s.Run.AuthorizeTx(ctx, tx, actor, "production.deploy", res); e != nil {
			return e
		}
		var existing *string
		if e := registry.Lookup(ctx, tx, "SELECT deployment_id FROM package_stages WHERE workspace_id=?", []any{id}, &existing); e != nil {
			return e
		}
		if existing != nil {
			if actor.Agent != nil {
				actor.Agent.ResultRef["id"] = *existing
			}
			return nil
		}
		if e := s.Run.Access.ProductionMutationGuard(ctx, tx, actor.Principal, "production.deploy", res); e != nil {
			return e
		}
		p, e := s.frozen(ctx, tx, id)
		if e != nil {
			return e
		}
		if e = s.productionSpace(ctx, tx, tenant, v.Space); e != nil {
			return e
		}
		base, e := s.Delivery.ProductionConfiguration(ctx, tx, v.Space)
		if e != nil {
			return e
		}
		if entity.Hash(base) != p.Base {
			return registry.Conflict("Production changed since review. Discard the stage and plan again.")
		}
		if e = conflicts(ctx, tx, v.Space, p.Changes); e != nil {
			return e
		}
		if e = s.Delivery.CheckPackageWorkspace(ctx, tx, actor, id, p.Base, p.Changes); e != nil {
			return e
		}
		operation, e := s.Delivery.QueuePackage(ctx, tx, actor, res, v.Mode, p.Base, access.Deref(v.Release), p.Changes, p.Audience)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "UPDATE package_stages SET deployment_id=? WHERE workspace_id=?", operation, id)
		return e
	})
	if e != nil {
		return v, e
	}
	return s.Status(ctx, actor, tenant, id)
}

func (s *Service) Retry(ctx context.Context, actor op.Actor, tenant, id string, revision int64, reviewedBase ...string) (Stage, error) {
	v, e := s.Status(ctx, actor, tenant, id)
	if e != nil {
		return v, e
	}
	if v.Operation != nil {
		hash := ""
		if len(reviewedBase) > 0 {
			hash = reviewedBase[0]
		}
		plan, err := s.frozen(ctx, s.DB, id)
		if err != nil {
			return v, err
		}
		_, e = s.Delivery.RetryReviewedDeployment(ctx, actor, *v.Operation, revision, hash, func(ctx context.Context, tx *sql.Tx, base delivery.SpaceConfiguration) error {
			return s.validateRebase(ctx, tx, tenant, plan, base)
		})
	} else {
		_, e = s.Delivery.ChangeWorkspace(ctx, actor, id, true, revision)
	}
	if e != nil {
		return v, e
	}
	return s.Status(ctx, actor, tenant, id)
}

// Discard releases a package reservation after its unused private stage has
// been archived through the workspace lifecycle.
func (s *Service) Discard(ctx context.Context, actor op.Actor, tenant, id string) error {
	v, e := s.Status(ctx, actor, tenant, id)
	if e != nil {
		return e
	}
	if v.Operation != nil || v.State != "archived" {
		return registry.Conflict("Archive the unused workspace before discarding its plan.")
	}
	return s.Run.Mutate(ctx, actor, "workspace.manage", access.Resource{Tenant: tenant, Space: v.Space}, func(tx *sql.Tx) error {
		if e := databases.SchedulePreviewCleanup(ctx, tx, id); e != nil {
			return e
		}
		r, e := tx.ExecContext(ctx, "DELETE FROM package_stages WHERE workspace_id=? AND deployment_id IS NULL", id)
		if e != nil {
			return e
		}
		n, e := r.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			return registry.Conflict("The stage has already been activated.")
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM package_installations WHERE id=? AND NOT EXISTS(SELECT 1 FROM package_stages WHERE installation_id=?)", v.Installation, v.Installation)
		return e
	})
}

// ReviewRetry binds an explicit retry review to the current production base.
// It checks reserved rows and the current contracts without changing the queue.
func (s *Service) ReviewRetry(ctx context.Context, actor op.Actor, tenant, id string) (Plan, error) {
	v, e := s.Status(ctx, actor, tenant, id)
	if e != nil {
		return Plan{}, e
	}
	if v.Operation == nil || v.State != "failed" {
		return Plan{}, registry.Conflict("Select a failed production installation.")
	}
	if e = s.Run.Authorize(ctx, actor, "production.deploy", access.Resource{Tenant: tenant, Space: v.Space}); e != nil {
		return Plan{}, e
	}
	if actor.Principal.Agent != nil {
		if e = access.RequireAgent(actor.Principal.Agent, "production.deploy"); e != nil {
			return Plan{}, e
		}
	}
	p, e := s.frozen(ctx, s.DB, id)
	if e != nil {
		return p, e
	}
	base, e := s.Delivery.ProductionConfiguration(ctx, s.DB, v.Space)
	if e != nil {
		return p, e
	}
	if e = s.validateRebase(ctx, s.DB, tenant, p, base); e != nil {
		return p, e
	}
	p.Base = entity.Hash(base)
	p.InheritingApps = []string{}
	if len(p.Changes.Entities) > 0 {
		for _, a := range base.Apps {
			if a.State == registry.StateActive && a.Access.Inherit {
				p.InheritingApps = append(p.InheritingApps, a.Name)
			}
		}
	}
	return p, nil
}
