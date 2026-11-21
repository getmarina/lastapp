package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/authz"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/roles"
)

func (s *Server) agentRoot(p access.Principal, target *Target) (access.Resource, *Target, error) {
	g := p.Agent
	if target == nil {
		target = &Target{Kind: g.Kind, ID: g.Scope}
		if g.Kind == "tenant" {
			target.ID = g.Tenant
		}
	}
	// Normalize a private copy so omitted Tenant IDs cannot be reinterpreted as
	// platform scope by the shared HTTP operation, or mutate request-key input.
	copyTarget := *target
	target = &copyTarget
	res := access.Resource{Tenant: g.Tenant}
	switch target.Kind {
	case "platform":
		if g.Kind != "platform" || target.ID != "" {
			return res, nil, registry.Missing()
		}
	case "tenant":
		if registry.ID(g.Tenant) != nil || target.ID != "" && target.ID != g.Tenant {
			return res, nil, registry.Missing()
		}
		target.ID = g.Tenant
	case "space":
		if e := registry.ID(target.ID); e != nil {
			return res, nil, e
		}
		res.Space = target.ID
	case "app":
		if e := registry.ID(target.ID); e != nil {
			return res, nil, e
		}
		res.App = target.ID
	case "team":
		if e := registry.ID(target.ID); e != nil {
			return res, nil, e
		}
		res.Team = target.ID
	default:
		return res, nil, registry.Bad("Unsupported target kind.")
	}
	return res, target, nil
}

func agentPaths(res access.Resource, target *Target) map[string]string {
	v := map[string]string{"tenant": res.Tenant}
	switch target.Kind {
	case "space":
		v["space"] = target.ID
	case "app":
		v["app"] = target.ID
	case "team":
		v["team"] = target.ID
	}
	return v
}

// agentScope is the role/assignment scope of a normalized agent target, the
// one the matching console route would select.
func agentScope(res access.Resource, target *Target) (authz.Scope, access.Resource, error) {
	paths := agentPaths(res, target)
	return roles.ScopeOf(paths["tenant"], paths["team"], paths["space"], paths["app"])
}

func (s *Server) beginAgentWrite(ctx context.Context, p access.Principal, key string, result registry.Row, input any, permissions ...string) (context.Context, error) {
	if e := registry.ID(key); e != nil {
		return ctx, registry.Bad("request_key must be a canonical UUID")
	}
	if e := access.RequireAgent(p.Agent, permissions...); e != nil {
		return ctx, e
	}
	old := access.AgentCallFrom(ctx)
	operation := ""
	if old != nil {
		operation = old.Operation
	}
	a := old
	if a == nil {
		a = &access.AgentCall{}
	}
	*a = access.AgentCall{Grant: p.Agent, Permission: permissions, Operation: operation, Key: key, Hash: entity.Hash(registry.Row{"operation": operation, "input": input}), ResultRef: result}
	var previous string
	e := s.DB.QueryRowContext(ctx, "SELECT input_hash FROM agent_operation_receipts WHERE grant_id=? AND request_key=?", p.Agent.ID, key).Scan(&previous)
	if e == nil {
		if previous != a.Hash {
			return ctx, registry.Conflict("request_key already belongs to different arguments")
		}
		return context.WithValue(ctx, access.AgentContextKey{}, a), nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return ctx, e
	}
	return context.WithValue(ctx, access.AgentContextKey{}, a), nil
}

// agentFits enforces the MCP response bound on an operation result, measured
// as its serialized JSON (with the encoder's trailing newline).
func agentFits(v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if len(b)+1 > MaxResponse {
		return registry.Problem(413, "response_limit", "Response exceeds the context limit. Use a smaller page or entity section; for schemas, select one field.")
	}
	return nil
}

// agentSized passes a successful operation result through agentFits.
func agentSized[T any](v T, e error) (any, error) {
	if e != nil {
		return nil, e
	}
	if e = agentFits(v); e != nil {
		return nil, e
	}
	return v, nil
}

func agentWriteResult(ctx context.Context, data any, e error) (any, error) {
	if errors.Is(e, op.ErrAgentReplay) {
		a := access.AgentCallFrom(ctx)
		a.Committed = true
		return registry.Row{"replayed": true, "committed": true, "resource": a.ResultRef, "next": "Inspect the resource/job with lastapp_get; this request was not executed twice."}, nil
	}
	var ae *registry.APIError
	if errors.As(e, &ae) && ae.Code == "response_limit" {
		// The operation committed but its result exceeds the response bound.
		// The receipt is the authoritative, compact outcome.
		if a := access.AgentCallFrom(ctx); a != nil && a.Committed {
			return registry.Row{"committed": true, "resource": a.ResultRef, "response_error": registry.Row{"code": ae.Code, "message": ae.Message}, "next": "Inspect with lastapp_get. For an entity use section=definition and version=draft; request individual schema sections/fields as needed. Unchanged retries use the same request_key."}, nil
		}
	}
	return data, e
}

// StableID derives the record ID a write with request key creates.
func StableID(p access.Principal, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("lastapp:agent:"+p.Agent.ID+":"+key)).String()
}
