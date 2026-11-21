package apppackages

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entities"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/registry"
)

func (s *Service) validateRebase(ctx context.Context, q registry.Queryer, tenant string, p Plan, base delivery.SpaceConfiguration) error {
	actual := map[string]bool{}
	for _, d := range base.Entities {
		actual[d.ID] = d.State == registry.StateActive
	}
	dependency := func(pkg, key string) (string, error) {
		id := itemID(p.Space+":"+pkg, "entity", key)
		if !actual[id] {
			return "", registry.Conflict("A package dependency is no longer available: " + pkg + "/" + key)
		}
		return id, nil
	}
	v, e := s.lookup(ctx, q, p.Key, p.Version)
	if e != nil {
		return e
	}
	if p.Mode == Attach {
		expected, e := v.Manifest.configuration(p.Space+":"+p.Key, true, false, dependency)
		if e != nil {
			return e
		}
		if e = compatible(expected, base); e != nil {
			return e
		}
	}
	missingEntities := 0
	for _, d := range p.Changes.Entities {
		var id, state string
		var draft registry.JSONValue
		e = q.QueryRowContext(ctx, "SELECT id,state,draft_json FROM entity_definitions WHERE space_id=? AND entity_key=?", p.Space, d.Definition.Key).Scan(&id, &state, &draft)
		if errors.Is(e, sql.ErrNoRows) {
			missingEntities++
			continue
		}
		if e != nil {
			return e
		}
		stored, e := entities.DefOf(draft.Value())
		if e != nil || id != d.ID || state != registry.StateArchived || entity.Hash(stored) != entity.Hash(d.Definition) {
			return registry.Conflict("The destination now contains a different entity: " + d.Definition.Key)
		}
	}
	if e = entities.CheckSpaceCapacity(ctx, q, p.Space, missingEntities); e != nil {
		return e
	}
	for _, r := range p.Changes.Roles {
		owner := "s:" + p.Space
		if r.App != "" {
			owner = "a:" + r.App
		}
		var id, state, name string
		e = q.QueryRowContext(ctx, "SELECT id,state,display_name FROM roles WHERE owner_key=? AND code=?", owner, r.Code).Scan(&id, &state, &name)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		if id != r.ID || state != registry.StateDisabled || name != r.Name {
			return registry.Conflict("The destination now contains a different role: " + r.Code)
		}
	}
	for _, a := range p.Changes.Apps {
		var id, state, name string
		var active *string
		e = q.QueryRowContext(ctx, "SELECT id,state,display_name,active_release_id FROM apps WHERE space_id=? AND slug=?", p.Space, a.Slug).Scan(&id, &state, &name, &active)
		if errors.Is(e, sql.ErrNoRows) {
			return registry.Conflict("The reserved App is missing.")
		}
		if e != nil {
			return e
		}
		if id != a.ID || state != registry.StateArchived || name != a.Name || active != nil {
			return registry.Conflict("The destination now contains a different App: " + a.Slug)
		}
	}
	return s.Delivery.ConfigurationQuota(ctx, q, tenant, delivery.SpaceConfiguration{Apps: p.Changes.Apps})
}
