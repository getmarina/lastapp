package agent

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/apppackages"
	"github.com/aantich/lastapp/internal/httpapi"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

type PackageReadInput struct {
	Release   string                 `json:"release_id,omitempty"`
	Action    string                 `json:"action" jsonschema:"catalog, inspect, plan, stages, status, retry_plan or source"`
	Key       string                 `json:"key,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Stage     string                 `json:"workspace_id,omitempty"`
	Selection *apppackages.Selection `json:"selection,omitempty"`
	Page      int                    `json:"page,omitempty"`
}

type PackageWriteInput struct {
	ReviewedBase string                  `json:"base_hash,omitempty" jsonschema:"Reviewed production base for a retry after an intervening change."`
	Action       string                  `json:"action" jsonschema:"stage, preview, activate, retry or discard"`
	Stage        string                  `json:"workspace_id,omitempty"`
	Plan         *apppackages.StageInput `json:"plan,omitempty"`
	Revision     int64                   `json:"expected_revision,omitempty"`
	RequestKey   string                  `json:"request_key"`
}

func (s *Server) toolPackageRead(ctx context.Context, p access.Principal, raw json.RawMessage) (any, error) {
	var in PackageReadInput
	if e := strictAgentInput(raw, &in); e != nil {
		return nil, e
	}
	a, tenant := op.AgentActor(ctx, p), p.Agent.Tenant
	page := op.PageRequest{Page: in.Page, Limit: 20}
	switch in.Action {
	case "catalog":
		return agentSized(s.Packages.Catalog(ctx, a, tenant, page))
	case "inspect":
		return agentSized(s.Packages.Inspect(ctx, a, tenant, in.Key, in.Version))
	case "plan":
		if in.Selection != nil {
			return agentSized(s.Packages.Plan(ctx, a, tenant, *in.Selection))
		}
	case "stages":
		return agentSized(s.Packages.Stages(ctx, a, tenant, page))
	case "status":
		return agentSized(s.Packages.Status(ctx, a, tenant, in.Stage))
	case "retry_plan":
		return agentSized(s.Packages.ReviewRetry(ctx, a, tenant, in.Stage))
	case "source":
		id, e := s.Delivery.SourceReference(ctx, a, in.Release)
		if e != nil {
			return nil, e
		}
		if id == "" {
			return registry.Row{"available": false}, nil
		}
		return registry.Row{"available": true, "download_url": s.Config.PublicURL + "/mcp/sources/" + id}, nil
	}
	return nil, registry.Bad("Choose a package read action and its required arguments.")
}

func (s *Server) toolPackageWrite(ctx context.Context, p access.Principal, raw json.RawMessage) (any, error) {
	var in PackageWriteInput
	if e := strictAgentInput(raw, &in); e != nil {
		return nil, e
	}
	tenant := p.Agent.Tenant
	id := in.Stage
	permission := "workspaces.write"
	var call func(context.Context, op.Actor) (any, error)
	switch in.Action {
	case "stage":
		if in.Plan == nil {
			return nil, registry.Bad("Reviewed plan required.")
		}
		id = StableID(p, in.RequestKey)
		call = func(ctx context.Context, a op.Actor) (any, error) { return s.Packages.Stage(ctx, a, tenant, *in.Plan) }
	case "preview":
		permission = "releases.write"
		call = func(ctx context.Context, a op.Actor) (any, error) { return s.Packages.Preview(ctx, a, tenant, id) }
	case "activate":
		permission = "production.deploy"
		call = func(ctx context.Context, a op.Actor) (any, error) { return s.Packages.Activate(ctx, a, tenant, id) }
	case "retry":
		stage, e := s.Packages.Status(ctx, op.AgentActor(ctx, p), tenant, id)
		if e != nil {
			return nil, e
		}
		if stage.Operation != nil {
			permission = "production.deploy"
		}
		call = func(ctx context.Context, a op.Actor) (any, error) {
			return s.Packages.Retry(ctx, a, tenant, id, in.Revision, in.ReviewedBase)
		}
	case "discard":
		call = func(ctx context.Context, a op.Actor) (any, error) {
			return registry.Row{"id": id}, s.Packages.Discard(ctx, a, tenant, id)
		}
	default:
		return nil, registry.Bad("Choose a package mutation action.")
	}
	if registry.ID(id) != nil {
		return nil, registry.Bad("A valid workspace_id is required.")
	}
	result := registry.Row{"id": id, "operation": "package." + in.Action}
	if in.Action == "activate" {
		result["id"] = StableID(p, in.RequestKey)
		result["workspace_id"] = id
	}
	ctx, e := s.beginAgentWrite(ctx, p, in.RequestKey, result, in, permission)
	if e != nil {
		return nil, e
	}
	v, e := agentSized(call(ctx, op.AgentActor(ctx, p)))
	return agentWriteResult(ctx, v, e)
}

func (s *Server) sourceDownload(w http.ResponseWriter, r *http.Request) {
	if !s.admitAgent(w, r, &s.validationLimiter, s.ValidationSlots, "agent-source", 30, 4) {
		return
	}
	ctx, cancel := op.WithRequestDeadline(r.Context())
	defer cancel()
	p, e := s.Principal(r.WithContext(ctx))
	<-s.ValidationSlots
	if e == nil {
		e = access.RequireAgent(p.Agent, "read")
	}
	if e == nil {
		e = s.Limiter.LimitRequest(w, r, s.Checker, s.DB, p)
	}
	if e == nil {
		select {
		case s.SourceSlots <- struct{}{}:
			defer func() { <-s.SourceSlots }()
		default:
			httpapi.WriteError(w, r, httpapi.RateLimited(w))
			return
		}
		f, err := s.Delivery.ReleaseSource(ctx, op.AgentActor(ctx, p), r.PathValue("release"))
		e = err
		if e == nil {
			defer f.Close()
			stat, err := f.Stat()
			e = err
			if e == nil {
				deadline, _ := ctx.Deadline()
				_ = http.NewResponseController(w).SetWriteDeadline(deadline)
				w.Header().Set("Cache-Control", "private, no-store")
				w.Header().Set("Content-Type", "application/zip")
				w.Header().Set("Content-Disposition", "attachment; filename=lastapp-source.zip")
				http.ServeContent(w, r, "lastapp-source.zip", stat.ModTime(), f)
				return
			}
		}
	}
	httpapi.WriteError(w, r, e)
}
