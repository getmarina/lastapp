// Package entities owns Space entity drafts, checks and tests, publication
// requests, and App access and contracts derived from published entities.
package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/config"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/inventory"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/roles"
)

// EntityDefinition is a Space entity: its draft, lifecycle and active and
// pending publications. The authoring view (Entity) adds the optional keys.
type EntityDefinition struct {
	ActiveDefinition  *registry.JSONValue  `json:"active_definition,omitempty"`
	ActiveVersion     *string              `json:"active_version_id"`
	ActiveNumber      *int64               `json:"active_version_number,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	Diagnostics       *[]entity.Diagnostic `json:"diagnostics,omitempty"`
	DraftContract     registry.Row         `json:"draft_contract,omitempty"`
	Draft             registry.JSONValue   `json:"draft_json"`
	Key               string               `json:"entity_key"`
	ID                string               `json:"id"`
	Label             string               `json:"label"`
	NextNumber        *int64               `json:"next_version_number,omitempty"`
	PendingVersion    *string              `json:"pending_version_id"`
	PendingNumber     *int64               `json:"pending_version_number,omitempty"`
	Publications      *[]PublicationItem   `json:"publications,omitempty"`
	PublishedContract registry.Row         `json:"published_contract,omitempty"`
	Revision          int64                `json:"revision"`
	SpaceID           string               `json:"space_id"`
	State             string               `json:"state"`
	TenantID          string               `json:"tenant_id"`
}

const EntityColumns = "active_version_id,created_at,draft_json,entity_key,id,label,pending_version_id,revision,space_id,state,tenant_id"

func (v *EntityDefinition) ScanFields() []any {
	return []any{&v.ActiveVersion, &v.CreatedAt, &v.Draft, &v.Key, &v.ID, &v.Label, &v.PendingVersion, &v.Revision, &v.SpaceID, &v.State, &v.TenantID}
}

// draft decodes the saved draft definition.
func (v EntityDefinition) DraftDefinition() (entity.Definition, error) { return DefOf(v.Draft.Value()) }

// EntityItem is an entity in a Space's entity collection. HasDraftChanges
// says whether the saved draft differs from the active version: nothing is
// published, or the draft's definition hash differs from the active one's.
type EntityItem struct {
	ActiveVersion   *string `json:"active_version_id"`
	ActiveNumber    *int64  `json:"active_version_number"`
	Key             string  `json:"entity_key"`
	HasDraftChanges bool    `json:"has_draft_changes"`
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	PendingVersion  *string `json:"pending_version_id"`
	Revision        int64   `json:"revision"`
	State           string  `json:"state"`
	draft           []byte
	activeHash      *string
}

const entityItemColumns = "e.active_version_id,v.version_number,e.entity_key,e.id,e.label,e.pending_version_id,e.revision,e.state,e.draft_json,v.definition_hash"

func (v *EntityItem) ScanFields() []any {
	return []any{&v.ActiveVersion, &v.ActiveNumber, &v.Key, &v.ID, &v.Label, &v.PendingVersion, &v.Revision, &v.State, &v.draft, &v.activeHash}
}

// draftChanged compares the draft's definition hash, computed as publication
// computes it (entity.Hash of the decoded definition), with the active
// version's. A draft that does not decode counts as changed.
func (v *EntityItem) draftChanged() bool {
	if v.activeHash == nil {
		return true
	}
	d, e := entity.Decode(v.draft)
	return e != nil || entity.Hash(d) != *v.activeHash
}

// NextVersionNumber selects the number the entity's next version gets
// (argument: entity ID). Each entity numbers its versions 1, 2, ... in creation
// order; every created version takes one whatever its state, a failed version
// keeps its number (published numbers can have gaps) and versions are never
// deleted, so a number is never reused. Assignment appends FOR UPDATE under the
// entity row lock; the unique (entity_id, version_number) key backs it.
const NextVersionNumber = "SELECT COALESCE(MAX(version_number),0)+1 FROM entity_versions WHERE entity_id=?"

// PublicationItem is one of an entity's recent publications.
type PublicationItem struct {
	BaseVersion *string   `json:"base_version_id"`
	CreatedAt   time.Time `json:"created_at"`
	Hash        string    `json:"definition_hash"`
	Error       *string   `json:"error_message"`
	ID          string    `json:"id"`
	State       string    `json:"state"`
	Number      int64     `json:"version_number"`
}

func (v *PublicationItem) ScanFields() []any {
	return []any{&v.BaseVersion, &v.CreatedAt, &v.Hash, &v.Error, &v.ID, &v.State, &v.Number}
}

// Publication is an entity publication with its completion time.
type Publication struct {
	BaseVersion *string    `json:"base_version_id"`
	CreatedAt   time.Time  `json:"created_at"`
	Hash        string     `json:"definition_hash"`
	Error       *string    `json:"error_message"`
	FinishedAt  *time.Time `json:"finished_at"`
	ID          string     `json:"id"`
	State       string     `json:"state"`
	Number      int64      `json:"version_number"`
}

func (v *Publication) ScanFields() []any {
	return []any{&v.BaseVersion, &v.CreatedAt, &v.Hash, &v.Error, &v.FinishedAt, &v.ID, &v.State, &v.Number}
}

// versionNumber is a version's number, or nil for no version.
func versionNumber(ctx context.Context, q registry.Queryer, version *string) (*int64, error) {
	if version == nil {
		return nil, nil
	}
	var n int64
	if e := q.QueryRowContext(ctx, "SELECT version_number FROM entity_versions WHERE id=?", *version).Scan(&n); e != nil {
		return nil, e
	}
	return &n, nil
}

// storedDefinition is a stored definition document.
type storedDefinition struct{ Definition registry.JSONValue }

func (v *storedDefinition) ScanFields() []any { return []any{&v.Definition} }

// publishedEntity is an active entity with its published definition.
type publishedEntity struct {
	ID, ActiveVersion, Hash string
	Number                  int64
	Definition              registry.JSONValue
}

func (v *publishedEntity) ScanFields() []any {
	return []any{&v.ID, &v.ActiveVersion, &v.Hash, &v.Number, &v.Definition}
}

// VersionDefinition reads a stored entity version's definition document.
func VersionDefinition(ctx context.Context, q registry.Queryer, version string) (registry.JSONValue, error) {
	var v storedDefinition
	return v.Definition, registry.Lookup(ctx, q, "SELECT definition_json FROM entity_versions WHERE id=?", []any{version}, &v.Definition)
}

func ValidateEntityRefs(ctx context.Context, q registry.Queryer, d entity.Definition, v EntityDefinition) error {
	return ValidateEntityRefsCandidates(ctx, q, d, v.TenantID, v.SpaceID, v.ID, nil)
}

func ValidateEntityRefsCandidates(ctx context.Context, q registry.Queryer, d entity.Definition, tenant, space, entityID string, candidates map[string]bool) error {
	for _, p := range d.Permissions {
		for _, rid := range p.Roles {
			if e := registry.ID(rid); e != nil {
				return e
			}
			var n int
			if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM roles WHERE id=? AND tenant_id=? AND space_id=? AND app_id IS NULL AND domain='data' AND (state='active' OR ?=1)", rid, tenant, space, candidates != nil).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				return registry.Bad("A permission references an unavailable Space business role.")
			}
		}
		if p.Subject != "" {
			if e := registry.ID(p.Subject); e != nil {
				return e
			}
			var n int
			// Captured batches copy already-published ACLs. A later suspension
			// must keep that reference inert, rather than making copying fail.
			// Live actor resolution still excludes inactive users and teams.
			query := "SELECT COUNT(*) FROM groups WHERE id=? AND tenant_id=? AND (state='active' OR ?=1)"
			if p.Relationship == "user" {
				query = "SELECT COUNT(*) FROM tenant_memberships m JOIN users u ON u.id=m.user_id WHERE m.user_id=? AND m.tenant_id=? AND ((u.state='active' AND m.state='active') OR ?=1)"
			}
			if e := q.QueryRowContext(ctx, query, p.Subject, tenant, candidates != nil).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				return registry.Bad("Unavailable permission recipient.")
			}
		}
	}
	for _, f := range d.Fields {
		if f.Type == "reference" {
			if e := registry.ID(f.Target); e != nil {
				return e
			}
		}
		if f.Type == "reference" && f.Target != entityID && !candidates[f.Target] {
			var n int
			if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM entity_definitions WHERE id=? AND tenant_id=? AND space_id=? AND state='active' AND active_version_id IS NOT NULL", f.Target, tenant, space).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				return registry.Bad("Reference target must be a published entity in this Space.")
			}
		}
	}
	return nil
}

// Reuse the canonical entity generators; selecting a section/field bounds agent
// context without inventing a second schema or exposing App-hidden rules/ACLs.
func entitySection(d entity.Definition, section, field string) (any, error) {
	if section == "definition" {
		if field != "" {
			return nil, registry.Bad("field applies only to generated contract sections.")
		}
		return d, nil
	}
	if field != "" {
		found := false
		for _, f := range d.Fields {
			if f.Key == field {
				d.Fields = []entity.Field{f}
				found = true
				break
			}
		}
		if !found {
			return nil, registry.Missing()
		}
	}
	switch section {
	case "json_schema":
		return d.JSONSchema(), nil
	case "create_schema":
		return d.Schema("create"), nil
	case "update_schema":
		return d.Schema("update"), nil
	case "typescript":
		return d.TypeScript(), nil
	default:
		return nil, registry.Bad("Unknown entity section.")
	}
}

func fieldInventory(fields []entity.Field) []registry.Row {
	out := []registry.Row{}
	for _, f := range fields {
		out = append(out, registry.Row{"key": f.Key, "label": f.Label, "type": f.Type, "target_entity_id": f.Target})
	}
	return out
}

// Authoring owns Space entity drafts, publication requests, policy checks and
// the App contracts and data access derived from them. Console handlers and
// MCP tools both call these methods.
type Authoring struct {
	DB     *sql.DB
	Run    *op.Runner
	Config *config.Config
	Queue  func(ctx context.Context, tx *sql.Tx, p access.Principal, v EntityDefinition, version string, candidates map[string]bool) error
}

type (
	EntityCreateInput struct {
		ID         string            `json:"id"`
		Definition entity.Definition `json:"definition"`
	}
	EntitySaveInput struct {
		Definition entity.Definition `json:"definition"`
		State      string            `json:"state"`
	}
	// EntityView selects a generated section of a draft or published
	// definition; the zero value is the full authoring view.
	EntityView struct {
		Section, Version, Field string
	}
	EntityTestInput struct {
		Definition entity.Definition `json:"definition"`
		App        string            `json:"app_id"`
		User       string            `json:"user_id"`
		Operation  string            `json:"operation"`
		Record     map[string]any    `json:"record"`
		Values     map[string]any    `json:"values"`
	}
	// ContractView selects App contracts: one entity, a generated section and
	// field, and the page of the inventory section.
	ContractView struct {
		Entity, Section, Field, Version string
		Page                            op.PageRequest
	}
)

// EntityRow reads an entity and checks action on its Space with the
// stricter mutating rules, as every entity operation does.
func (v *Authoring) EntityRow(ctx context.Context, q registry.Queryer, a op.Actor, key, action string) (EntityDefinition, error) {
	out, e := registry.ScanOne[EntityDefinition](ctx, q, "SELECT "+EntityColumns+" FROM entity_definitions WHERE id=?", key)
	if e != nil {
		return out, e
	}
	e = v.Run.Check(ctx, q, a, action, access.Resource{Tenant: out.TenantID, Space: out.SpaceID}, true)
	return out, e
}

// AuthoringSpace reads an active Space whose entities a may manage.
func (v *Authoring) AuthoringSpace(ctx context.Context, a op.Actor, space string) (access.Resource, error) {
	res := access.Resource{Space: space}
	if e := registry.Lookup(ctx, v.DB, "SELECT tenant_id FROM spaces WHERE id=? AND state='active'", []any{space}, &res.Tenant); e != nil {
		return res, e
	}
	return res, v.Run.Check(ctx, v.DB, a, "entity.manage", res, true)
}

func DefOf(v any) (entity.Definition, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return entity.Definition{}, e
	}
	return entity.Decode(b)
}

func (v *Authoring) Entities(ctx context.Context, a op.Actor, space string) (registry.Row, error) {
	sp, e := v.AuthoringSpace(ctx, a, space)
	if e != nil {
		return nil, e
	}
	items, e := registry.ScanAll[EntityItem](ctx, v.DB, "SELECT "+entityItemColumns+" FROM entity_definitions e LEFT JOIN entity_versions v ON v.id=e.active_version_id WHERE e.space_id=? AND e.tenant_id=? ORDER BY e.label,e.id", sp.Space, sp.Tenant)
	if e != nil {
		return nil, e
	}
	for i := range items {
		items[i].HasDraftChanges = items[i].draftChanged()
		items[i].draft, items[i].activeHash = nil, nil
	}
	return registry.Row{"items": items}, nil
}

// CreateEntity adds a draft to a Space and returns its authoring view.
func (v *Authoring) CreateEntity(ctx context.Context, a op.Actor, space string, in EntityCreateInput) (EntityDefinition, error) {
	if in.ID == "" {
		in.ID = registry.NewID()
	}
	if e := registry.ID(in.ID); e != nil {
		return EntityDefinition{}, e
	}
	sp, e := v.AuthoringSpace(ctx, a, space)
	if e != nil {
		return EntityDefinition{}, e
	}
	if in.Definition.Format != 1 || len(in.Definition.Key) > 48 || !entity.ValidKey(in.Definition.Key) || len(in.Definition.Label) > 160 || in.Definition.Label == "" {
		return EntityDefinition{}, registry.Bad("Provide an entity key and name.")
	}
	b, _ := json.Marshal(in.Definition)
	if len(b) > entity.MaxDefinitionBytes {
		return EntityDefinition{}, registry.Bad("Definition exceeds its encoded-size limit.")
	}
	tenant := sp.Tenant
	e = v.Run.Mutate(ctx, a, "entity.manage", sp, func(tx *sql.Tx) error {
		if e := CheckSpaceCapacity(ctx, tx, sp.Space, 1); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO entity_definitions(id,tenant_id,space_id,entity_key,label,draft_json) VALUES(?,?,?,?,?,?)", in.ID, sp.Tenant, sp.Space, in.Definition.Key, in.Definition.Label, b)
		if e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, a, tenant, "entity.created", "entity", in.ID, registry.Row{})
	})
	if e != nil {
		return EntityDefinition{}, e
	}
	return v.DetailOf(ctx, a, in.ID)
}

// Entity is the authoring view of a draft with its generated contracts and
// recent publications, or one selected section of it.
func (v *Authoring) Entity(ctx context.Context, a op.Actor, key string, view EntityView) (any, error) {
	out, e := v.EntityRow(ctx, v.DB, a, key, "entity.manage")
	if e != nil {
		return nil, e
	}
	if view.Section != "" {
		version := view.Version
		if version == "" {
			version = "draft"
		}
		definition := out.Draft
		if version == "published" {
			if out.ActiveVersion == nil {
				return nil, registry.Missing()
			}
			active, err := VersionDefinition(ctx, v.DB, *out.ActiveVersion)
			if err != nil {
				return nil, err
			}
			definition = active
		} else if version != "draft" {
			return nil, registry.Bad("version must be draft or published.")
		}
		d, err := DefOf(definition.Value())
		if err != nil {
			return nil, err
		}
		value, err := entitySection(d, view.Section, view.Field)
		if err != nil {
			return nil, err
		}
		return registry.Row{"id": out.ID, "space_id": out.SpaceID, "revision": out.Revision, "version": version, "active_version_id": out.ActiveVersion, "definition_hash": entity.Hash(d), "section": view.Section, "field": view.Field, "partial": view.Field != "", "value": value}, nil
	}
	if view.Field != "" || view.Version != "" {
		return nil, registry.Bad("field/version require a section.")
	}
	return v.entityDetail(ctx, out)
}

// DetailOf reads the full authoring view of an entity.
func (v *Authoring) DetailOf(ctx context.Context, a op.Actor, key string) (EntityDefinition, error) {
	out, e := v.EntityRow(ctx, v.DB, a, key, "entity.manage")
	if e != nil {
		return out, e
	}
	return v.entityDetail(ctx, out)
}

// entityDetail adds an entity's diagnostics, generated contracts and recent
// publications: the full authoring view.
func (v *Authoring) entityDetail(ctx context.Context, out EntityDefinition) (EntityDefinition, error) {
	d, e := out.DraftDefinition()
	if e != nil {
		return out, e
	}
	diagnostics := d.Validate()
	out.Diagnostics = &diagnostics
	out.DraftContract = registry.Row{"definition_hash": entity.Hash(d), "json_schema": d.JSONSchema(), "create_schema": d.Schema("create"), "update_schema": d.Schema("update"), "typescript": d.TypeScript()}
	versions, e := registry.ScanAll[PublicationItem](ctx, v.DB, "SELECT base_version_id,created_at,definition_hash,error_message,id,state,version_number FROM entity_versions WHERE entity_id=? ORDER BY version_number DESC LIMIT 20", out.ID)
	if e != nil {
		return out, e
	}
	out.Publications = &versions
	var next int64
	if e = v.DB.QueryRowContext(ctx, NextVersionNumber, out.ID).Scan(&next); e != nil {
		return out, e
	}
	out.NextNumber = &next
	if out.ActiveNumber, e = versionNumber(ctx, v.DB, out.ActiveVersion); e != nil {
		return out, e
	}
	if out.PendingNumber, e = versionNumber(ctx, v.DB, out.PendingVersion); e != nil {
		return out, e
	}
	if out.ActiveVersion != nil {
		active, e := VersionDefinition(ctx, v.DB, *out.ActiveVersion)
		if e != nil {
			return out, e
		}
		out.ActiveDefinition = &active
		published, e := DefOf(active.Value())
		if e != nil {
			return out, e
		}
		out.PublishedContract = registry.Row{"version_id": out.ActiveVersion, "definition_hash": entity.Hash(published), "json_schema": published.JSONSchema(), "create_schema": published.Schema("create"), "update_schema": published.Schema("update"), "typescript": published.TypeScript()}
	}
	return out, nil
}

// SaveEntity replaces a draft, optionally archiving or reactivating it, and
// returns its authoring view.
func (v *Authoring) SaveEntity(ctx context.Context, a op.Actor, key string, in EntitySaveInput, revision int64) (EntityDefinition, error) {
	if revision < 1 {
		return EntityDefinition{}, op.RevisionRequired()
	}
	old, e := v.EntityRow(ctx, v.DB, a, key, "entity.manage")
	if e != nil {
		return EntityDefinition{}, e
	}
	if in.Definition.Key != old.Key || in.Definition.Format != 1 || in.Definition.Label == "" || len(in.Definition.Label) > 160 {
		return EntityDefinition{}, registry.Bad("Entity key/format are immutable and a name is required.")
	}
	b, _ := json.Marshal(in.Definition)
	if len(b) > entity.MaxDefinitionBytes {
		return EntityDefinition{}, registry.Bad("Definition exceeds its encoded-size limit.")
	}
	tenant := old.TenantID
	e = v.Run.Mutate(ctx, a, "entity.manage", access.Resource{Tenant: tenant, Space: old.SpaceID}, func(tx *sql.Tx) error {
		var current int64
		var pending *string
		e := registry.Lookup(ctx, tx, "SELECT revision,pending_version_id FROM entity_definitions WHERE id=? FOR UPDATE", []any{old.ID}, &current, &pending)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(current, revision); e != nil {
			return e
		}
		state := old.State
		if in.State != "" {
			if !slices.Contains([]string{registry.StateActive, registry.StateArchived}, in.State) {
				return registry.Bad("Invalid lifecycle state.")
			}
			if pending != nil {
				return registry.Conflict("Wait for publishing before archiving.")
			}
			if in.State == registry.StateArchived {
				refs, e := registry.ScanAll[storedDefinition](ctx, tx, "SELECT v.definition_json FROM entity_definitions e JOIN entity_versions v ON v.id=e.active_version_id WHERE e.space_id=? AND e.state='active' AND e.id<>?", old.SpaceID, old.ID)
				if e != nil {
					return e
				}
				for _, ref := range refs {
					d, e := DefOf(ref.Definition.Value())
					if e != nil {
						return e
					}
					for _, f := range d.Fields {
						if f.Target == old.ID {
							return registry.Conflict("An active entity references this entity.")
						}
					}
				}
			}
			state = in.State
		}
		_, e = tx.ExecContext(ctx, "UPDATE entity_definitions SET label=?,draft_json=?,state=?,revision=revision+1 WHERE id=?", in.Definition.Label, b, state, old.ID)
		if e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, a, tenant, "entity.saved", "entity", old.ID, registry.Row{"state": state})
	})
	if e != nil {
		return EntityDefinition{}, e
	}
	return v.DetailOf(ctx, a, key)
}

// ValidateEntity checks a candidate definition without saving it. The
// diagnostics view omits the generated schema and TypeScript.
func (v *Authoring) ValidateEntity(ctx context.Context, a op.Actor, key string, d entity.Definition, view string) (registry.Row, error) {
	current, e := v.EntityRow(ctx, v.DB, a, key, "entity.manage")
	if e != nil {
		return nil, e
	}
	if view != "" && view != "diagnostics" {
		return nil, registry.Bad("Unknown validation view.")
	}
	diags := d.Validate()
	if e = ValidateEntityRefs(ctx, v.DB, d, current); e != nil {
		diags = append(diags, entity.Diagnostic{Code: "invalid_reference", Path: "permissions", Message: e.Error()})
	}
	out := registry.Row{"diagnostics": diags}
	if view != "diagnostics" {
		out["schema"], out["typescript"] = d.JSONSchema(), d.TypeScript()
	}
	return out, nil
}

// UnchangedDraft rejects publishing a draft identical to the active version.
const UnchangedDraft = "The draft matches the published version. Change the draft before publishing."

// PublishEntity queues the saved draft for the publication worker. A draft
// identical to the active version is rejected (409): it would only add a
// version that changes nothing.
func (v *Authoring) PublishEntity(ctx context.Context, a op.Actor, key string, revision int64) (registry.Row, error) {
	if revision < 1 {
		return nil, op.RevisionRequired()
	}
	current, e := v.EntityRow(ctx, v.DB, a, key, "entity.publish")
	if e != nil {
		return nil, e
	}
	version := a.OperationID()
	e = v.Run.Mutate(ctx, a, "entity.publish", access.Resource{Tenant: current.TenantID, Space: current.SpaceID}, func(tx *sql.Tx) error {
		current, e = registry.ScanOne[EntityDefinition](ctx, tx, "SELECT "+EntityColumns+" FROM entity_definitions WHERE id=? FOR UPDATE", current.ID)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(current.Revision, revision); e != nil {
			return e
		}
		if current.PendingVersion != nil {
			return registry.Conflict("A publication is already pending.")
		}
		var queued int
		if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM entity_versions WHERE tenant_id=? AND state IN ('queued','publishing','prepared')", current.TenantID).Scan(&queued); e != nil {
			return e
		}
		if queued >= 16 {
			return registry.Conflict("Publication queue limit reached. Wait for queued changes to finish.")
		}
		if current.State != registry.StateActive {
			return registry.Conflict("Archived entities cannot publish.")
		}
		// A version must change something: the draft's definition hash,
		// computed as publication computes it, differs from the active one's.
		if current.ActiveVersion != nil {
			var active string
			if e = tx.QueryRowContext(ctx, "SELECT definition_hash FROM entity_versions WHERE id=?", *current.ActiveVersion).Scan(&active); e != nil {
				return e
			}
			if d, e := current.DraftDefinition(); e == nil && entity.Hash(d) == active {
				return registry.Conflict(UnchangedDraft)
			}
		}
		return v.Queue(ctx, tx, a.Principal, current, version, nil)
	})
	if e != nil {
		return nil, e
	}
	number, e := versionNumber(ctx, v.DB, &version)
	if e != nil {
		return nil, e
	}
	return registry.Row{"id": version, "state": "queued", "version_number": number}, nil
}

func (v *Authoring) Publication(ctx context.Context, a op.Actor, key, version string) (Publication, error) {
	current, e := v.EntityRow(ctx, v.DB, a, key, "entity.manage")
	if e != nil {
		return Publication{}, e
	}
	return registry.ScanOne[Publication](ctx, v.DB, "SELECT base_version_id,created_at,definition_hash,error_message,finished_at,id,state,version_number FROM entity_versions WHERE id=? AND entity_id=?", version, current.ID)
}

// TestEntity evaluates one synthetic case against a candidate definition
// for a real Tenant member, without live uniqueness, references or shares.
func (v *Authoring) TestEntity(ctx context.Context, a op.Actor, key string, in EntityTestInput) (registry.Row, error) {
	current, e := v.EntityRow(ctx, v.DB, a, key, "entity.manage")
	if e != nil {
		return nil, e
	}
	if ds := in.Definition.Validate(); len(ds) > 0 {
		return registry.Row{"diagnostics": ds, "allowed": false}, nil
	}
	if in.User == "" {
		in.User = a.Principal.ID
	}
	c, access, e := v.ActorContext(ctx, v.DB, in.User, current.TenantID, current.SpaceID, in.App)
	if e != nil {
		return nil, e
	}
	c.Record = in.Record
	c.Old = in.Record
	c.Now = time.Now().UTC()
	candidate := in.Record
	if in.Operation == "create" || in.Operation == "update" {
		recordID := registry.Str(in.Record["id"])
		if recordID == "" {
			recordID = registry.NewID()
		}
		revision := registry.Integer(in.Record["revision"]) + 1
		if in.Operation == "create" {
			revision = 1
		}
		candidate, e = in.Definition.Prepare(c, in.Values, in.Operation == "create", recordID, revision)
	}
	out := registry.Row{"allowed": false, "not_checked": []string{"live uniqueness", "reference existence/read authority", "record shares"}, "diagnostics": []any{}}
	if e != nil {
		out["error"] = e.Error()
		return out, nil
	}
	c.Record = candidate
	fields := []string{}
	if in.Operation == "read" {
		fields = in.Definition.Projection
	} else {
		for k := range in.Values {
			fields = append(fields, k)
		}
	}
	entityID := current.ID
	allowed := in.Definition.Allowed(entityID, in.Operation, fields, c, access, nil)
	if in.Operation == "read" {
		allowed = true
		for _, field := range fields {
			allowed = allowed && in.Definition.Allowed(entityID, "read", []string{field}, c, access, nil)
		}
	}
	out["allowed"] = allowed
	out["candidate"] = candidate
	explanation := []registry.Row{}
	for _, permission := range in.Definition.Permissions {
		if permission.Operation != in.Operation {
			continue
		}
		one := in.Definition
		one.Permissions = []entity.Permission{permission}
		explanation = append(explanation, registry.Row{"key": permission.Key, "label": permission.Label, "fields": permission.Fields, "matches_complete_path": one.Allowed(entityID, in.Operation, nil, c, access, nil)})
	}
	out["permission_paths"] = explanation
	if e = in.Definition.CheckRules(c, in.Operation == "create"); (in.Operation == "create" || in.Operation == "update") && e != nil {
		out["allowed"] = false
		out["error"] = e.Error()
	}
	return out, nil
}

// AppContracts are the published entity contracts an App may use: its
// access ceiling narrows fields and permissions unless a is a Space manager.
func (v *Authoring) AppContracts(ctx context.Context, a op.Actor, app string, view ContractView) (registry.Row, error) {
	current, e := inventory.AppRow(ctx, v.DB, app)
	if e != nil {
		return nil, e
	}
	if e = v.Run.Check(ctx, v.DB, a, "app.manage", access.Resource{Tenant: current.TenantID, App: current.ID}, true); e != nil {
		return nil, e
	}
	appAccess, e := registry.AccessOf(current.Access)
	if e != nil {
		return nil, e
	}
	manager := v.Run.Check(ctx, v.DB, a, "entity.manage", access.Resource{Tenant: current.TenantID, Space: current.SpaceID}, true) == nil
	vs, e := registry.ScanAll[publishedEntity](ctx, v.DB, "SELECT e.id,e.active_version_id,v.definition_hash,v.version_number,v.definition_json FROM entity_definitions e JOIN entity_versions v ON v.id=e.active_version_id WHERE e.space_id=? AND e.state='active' ORDER BY e.label,e.id", current.SpaceID)
	if e != nil {
		return nil, e
	}
	out := []registry.Row{}
	for _, value := range vs {
		if view.Entity != "" && view.Entity != value.ID {
			continue
		}
		d, e := DefOf(value.Definition.Value())
		if e != nil {
			return nil, e
		}
		perms := []registry.Row{}
		allowedFields := []string{}
		for _, permission := range d.Permissions {
			pid := entity.PermissionID(value.ID, permission.Key)
			if !manager && !appAccess.Inherit && !slices.ContainsFunc(appAccess.Ceiling, func(g entity.Gate) bool { return g.Permission == pid }) {
				continue
			}
			fields := permission.Fields
			if !manager && !appAccess.Inherit {
				fields = []string{}
				for _, g := range appAccess.Ceiling {
					if g.Permission == pid {
						fields = append(fields, g.Fields...)
					}
				}
			}
			allowedFields = append(allowedFields, fields...)
			perms = append(perms, registry.Row{"id": pid, "key": permission.Key, "label": permission.Label, "operation": permission.Operation, "fields": fields})
		}
		if len(perms) == 0 && !manager {
			continue
		}
		fields := []entity.Field{}
		for _, f := range d.Fields {
			if manager || slices.Contains(allowedFields, f.Key) {
				fields = append(fields, f)
			}
		}
		d.Fields = fields
		d.Permissions = nil
		d.Rules = nil
		section, field := view.Section, view.Field
		if view.Version != "" && view.Version != "published" {
			return nil, registry.Bad("App contracts are always published.")
		}
		if section == "inventory" {
			if field != "" {
				return nil, registry.Bad("field requires a generated contract section.")
			}
			out = append(out, registry.Row{"id": value.ID, "published_version_id": value.ActiveVersion, "published_version_number": value.Number, "definition_hash": value.Hash, "key": d.Key, "label": d.Label, "section": "inventory", "fields": fieldInventory(fields), "permissions": perms})
			continue
		}
		if section != "" {
			var part any
			if section == "definition" {
				if field != "" {
					return nil, registry.Bad("field applies only to generated contract sections.")
				}
				public, e := PublicEntityFields(fields)
				if e != nil {
					return nil, e
				}
				part = registry.Row{"key": d.Key, "label": d.Label, "fields": public, "permissions": perms}
			} else {
				part, e = entitySection(d, section, field)
				if e != nil {
					return nil, e
				}
			}
			out = append(out, registry.Row{"id": value.ID, "published_version_id": value.ActiveVersion, "published_version_number": value.Number, "definition_hash": value.Hash, "section": section, "field": field, "partial": field != "", "value": part})
			continue
		}
		if field != "" {
			return nil, registry.Bad("field requires a generated contract section.")
		}
		public, e := PublicEntityFields(fields)
		if e != nil {
			return nil, e
		}
		out = append(out, registry.Row{"id": value.ID, "published_version_id": value.ActiveVersion, "published_version_number": value.Number, "definition_hash": value.Hash, "definition": registry.Row{"key": d.Key, "label": d.Label, "fields": public, "permissions": perms}, "json_schema": d.JSONSchema(), "create_schema": d.Schema("create"), "update_schema": d.Schema("update"), "typescript": d.TypeScript()})
	}
	roles, e := registry.ScanAll[roles.Named](ctx, v.DB, "SELECT display_name,id FROM roles WHERE app_id=? AND domain='data' AND state='active'", current.ID)
	if e != nil {
		return nil, e
	}
	response := registry.Row{"app_id": current.ID, "space_id": current.SpaceID, "app_revision": current.Revision, "entities": out, "roles": roles}
	if view.Section == "inventory" {
		var page op.PageInfo
		out, page, e = op.PageItems(out, view.Page)
		if e != nil {
			return nil, e
		}
		response["entities"], response["pagination"] = out, page
	}
	return response, nil
}

// AppAccess is an App's data-access policy with its revision.
func (v *Authoring) AppAccess(ctx context.Context, a op.Actor, app string) (registry.Row, error) {
	current, e := inventory.AppRow(ctx, v.DB, app)
	if e != nil {
		return nil, e
	}
	if e = v.Run.Check(ctx, v.DB, a, "app.manage", access.Resource{Tenant: current.TenantID, App: current.ID}, true); e != nil {
		return nil, e
	}
	access, e := registry.AccessOf(current.Access)
	if e != nil {
		return nil, e
	}
	return registry.Row{"revision": current.Revision, "access": access}, nil
}

// SaveAppAccess replaces an App's origin, ceiling and restrictions. Only a
// Space manager may change the origin or widen the ceiling.
func (v *Authoring) SaveAppAccess(ctx context.Context, a op.Actor, app string, in entity.AppAccess, revision int64) (registry.Row, error) {
	if revision < 1 {
		return nil, op.RevisionRequired()
	}
	current, e := inventory.AppRow(ctx, v.DB, app)
	if e != nil {
		return nil, e
	}
	res := access.Resource{Tenant: current.TenantID, App: current.ID}
	e = v.Run.Mutate(ctx, a, "app.manage", res, func(tx *sql.Tx) error {
		current, e = inventory.AppRow(ctx, tx, current.ID)
		if e != nil {
			return e
		}
		if e = op.CheckRevision(current.Revision, revision); e != nil {
			return e
		}
		var wid sql.NullString
		if e = tx.QueryRowContext(ctx, "SELECT workspace_id FROM spaces WHERE id=?", current.SpaceID).Scan(&wid); e != nil {
			return e
		}
		workspace := wid.Valid
		old, e := registry.AccessOf(current.Access)
		if e != nil {
			return e
		}
		spaceManager := v.Run.AuthorizeTx(ctx, tx, a, "entity.manage", access.Resource{Tenant: res.Tenant, Space: current.SpaceID}) == nil
		if !spaceManager && (old.Origin != in.Origin || old.Inherit != in.Inherit || entity.Hash(old.Ceiling) != entity.Hash(in.Ceiling)) {
			return registry.Problem(403, "space_approval_required", "A Space manager must approve the origin and access ceiling.")
		}
		if in.Origin != "" {
			u, _ := url.Parse(in.Origin)
			admin, _ := url.Parse(v.Config.PublicURL)
			loopback := u != nil && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback())
			if !config.ValidOrigin(in.Origin, false) || u == nil || len(in.Origin) > 500 || u.Scheme != "https" && !((v.Config.Development || workspace) && loopback) || u.Hostname() != strings.ToLower(u.Hostname()) || strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), strings.TrimSuffix(admin.Hostname(), ".")) {
				return registry.Bad("App origin must use a separate host from the console.")
			}
		}
		if len(in.Ceiling)+len(in.Restrictions) > 128 {
			return registry.Bad("Too many App access gates.")
		}
		for _, gs := range [][]entity.Gate{in.Ceiling, in.Restrictions} {
			for _, g := range gs {
				if e := registry.ID(g.Permission); e != nil {
					return e
				}
				pv, e := registry.ScanOne[storedDefinition](ctx, tx, "SELECT v.definition_json FROM permissions p JOIN entity_versions v ON v.id=p.version_id WHERE p.id=? AND p.tenant_id=? AND p.space_id=? AND p.state='active' AND p.domain='data'", g.Permission, res.Tenant, current.SpaceID)
				if e != nil {
					return registry.Bad("Choose an active permission from this Space.")
				}
				d, e := DefOf(pv.Definition.Value())
				if e != nil {
					return e
				}
				for _, f := range g.Fields {
					if _, ok := d.Field(f); !ok {
						return registry.Bad("Unknown gate field.")
					}
				}
				if g.When != nil {
					if e = d.CheckCondition(*g.When, false); e != nil {
						return registry.Bad(e.Error())
					}
				}
				for _, role := range g.Roles {
					var n int
					if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM roles WHERE id=? AND tenant_id=? AND app_id=? AND domain='data' AND state='active'", role, res.Tenant, current.ID).Scan(&n); e != nil {
						return e
					}
					if n != 1 {
						return registry.Bad("App gates can reference only this App's business roles.")
					}
				}
			}
		}
		host := ""
		if in.Origin != "" && !workspace {
			parsed, _ := url.Parse(in.Origin)
			host = strings.ToLower(parsed.Hostname())
			var occupied int
			if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM apps WHERE origin_host=? AND id<>?", host, current.ID).Scan(&occupied); e != nil {
				return e
			}
			if occupied > 0 {
				return registry.Conflict("Each App needs a separate host. This host is already approved for another App.")
			}
		}
		b, _ := json.Marshal(in)
		_, e = tx.ExecContext(ctx, "UPDATE apps SET data_access_json=?,approved_origin=?,origin_host=?,origin_generation=origin_generation+IF(?<>?,1,0),revision=revision+1 WHERE id=?", b, registry.Nullable(in.Origin), registry.Nullable(host), old.Origin, in.Origin, current.ID)
		if e != nil {
			return e
		}
		if e = ProjectAppPermissions(ctx, tx, current.ID, in); e != nil {
			return e
		}
		return op.AuditAs(ctx, tx, a, res.Tenant, "app.data_access_changed", "app", current.ID, registry.Row{})
	})
	if e != nil {
		return nil, e
	}
	return v.AppAccess(ctx, a, app)
}

func ProjectAppPermissions(ctx context.Context, tx *sql.Tx, app string, a entity.AppAccess) error {
	if _, e := tx.ExecContext(ctx, "DELETE rp FROM role_permissions rp JOIN roles r ON r.id=rp.role_id WHERE r.app_id=? AND r.domain='data'", app); e != nil {
		return e
	}
	gates := []entity.Gate{}
	if !a.Inherit {
		gates = append(gates, a.Ceiling...)
	}
	if a.Restricted {
		gates = append(gates, a.Restrictions...)
	}
	for _, g := range gates {
		for _, role := range g.Roles {
			if _, e := tx.ExecContext(ctx, "INSERT IGNORE INTO role_permissions(role_id,permission_id,domain) VALUES(?,?,'data')", role, g.Permission); e != nil {
				return e
			}
		}
	}
	return nil
}

func PublicEntityFields(fields []entity.Field) ([]registry.Row, error) {
	out := []registry.Row{}
	for _, field := range fields {
		b, e := json.Marshal(field)
		if e != nil {
			return nil, e
		}
		m := registry.Row{}
		if e = json.Unmarshal(b, &m); e != nil {
			return nil, e
		}
		m["read_only"] = field.Calculation != nil
		m["has_default"] = field.Default != nil
		delete(m, "default")
		delete(m, "calculation")
		out = append(out, m)
	}
	return out, nil
}

// ActorContext resolves the live entity evaluation context of user in a
// Space/App: membership, scoped business roles, team relationships and the
// App's data-access narrowing.
func (v *Authoring) ActorContext(ctx context.Context, q registry.Queryer, user, tenant, space, app string) (entity.Context, entity.AppAccess, error) {
	c := entity.Context{Actor: user, Now: time.Now().UTC(), Roles: map[string]bool{}, Teams: map[string]bool{}, ReviewTeams: map[string]bool{}, AdminTeams: map[string]bool{}}
	a := entity.AppAccess{Inherit: true}
	if e := v.Run.Access.Member(ctx, q, access.Principal{ID: user}, tenant, true); e != nil {
		return c, a, e
	}
	query := strings.Replace(access.EffectiveAssignmentSQL, " ORDER BY g.scope_kind,g.id", " AND (g.scope_kind='team' OR (g.scope_kind='space' AND g.space_id=?) OR (g.scope_kind='app' AND g.app_id=?)) ORDER BY g.scope_kind,g.id", 1)
	assignments, e := registry.ScanAll[access.EffectiveAssignment](ctx, q, query, user, tenant, space, app)
	if e != nil {
		return c, a, e
	}
	for _, g := range assignments {
		if (g.ScopeKind == "space" && access.Deref(g.SpaceID) == space) || (g.ScopeKind == "app" && access.Deref(g.AppID) == app) {
			c.Roles[g.RoleID] = true
		}
		if g.ScopeKind == "team" {
			if g.Code == "team.reviewer" {
				c.ReviewTeams[access.Deref(g.ScopeTeamID)] = true
			}
			if g.Code == "team.admin" {
				c.AdminTeams[access.Deref(g.ScopeTeamID)] = true
			}
		}
	}
	teams, e := registry.ScanStrings(ctx, q, "SELECT gm.group_id FROM group_memberships gm JOIN groups g ON g.id=gm.group_id AND g.tenant_id=gm.tenant_id AND g.state='active' WHERE gm.tenant_id=? AND gm.user_id=? AND gm.state='active' LIMIT ?", tenant, user, entity.MaxActorTeams+1)
	if e != nil {
		return c, a, e
	}
	if len(teams) > entity.MaxActorTeams {
		return c, a, registry.Conflict("Membership expansion limit exceeded.")
	}
	for _, g := range teams {
		c.Teams[g] = true
	}
	if app != "" {
		var access registry.JSONValue
		if e := registry.Lookup(ctx, q, "SELECT data_access_json FROM apps WHERE id=? AND tenant_id=? AND space_id=? AND state='active'", []any{app, tenant, space}, &access); e != nil {
			return c, a, e
		}
		b, _ := json.Marshal(access)
		if e = json.Unmarshal(b, &a); e != nil {
			return c, a, e
		}
	}
	return c, a, nil
}

// BusinessRecord converts a stored business row to entity field values: the
// f_ column prefix is removed and integers, booleans and JSON are decoded.
func BusinessRecord(d entity.Definition, raw registry.Row) map[string]any {
	out := map[string]any{}
	for k, v := range raw {
		if strings.HasPrefix(k, "f_") {
			k = k[2:]
		} else if !slices.Contains([]string{"id", "revision", "created_at", "updated_at", "created_by", "updated_by"}, k) {
			continue
		}
		if f, ok := d.Field(k); ok && v != nil {
			switch f.Type {
			case "integer":
				v = registry.Integer(v)
			case "boolean":
				v = registry.Integer(v) != 0
			case "json":
				var obj any
				if json.Unmarshal([]byte(registry.Str(v)), &obj) == nil {
					v = obj
				}
			}
		}
		out[k] = v
	}
	return out
}

// CheckSpaceCapacity applies the same definition limit to ordinary authoring
// and bulk publication. Archived definitions still occupy their stable keys.
func CheckSpaceCapacity(ctx context.Context, q registry.Queryer, space string, added int) error {
	if added == 0 {
		return nil
	}
	var count int
	if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM entity_definitions WHERE space_id=?", space).Scan(&count); e != nil {
		return e
	}
	if count+added > entity.MaxSpaceEntities {
		return registry.Conflict("Space entity limit reached.")
	}
	return nil
}
