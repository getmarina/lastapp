package control

import (
	"context"
	"io"
	"net/http"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/apppackages"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/httpapi"
	"github.com/aantich/lastapp/internal/registry"
)

func (s *Server) PublishPackage(ctx context.Context, r io.Reader, b *artifact.Build, f []artifact.File, source []byte) (apppackages.Version, error) {
	return s.packages.Publish(ctx, r, b, f, source)
}

func (s *Server) packageRoutes() {
	const tenant = "/v1/tenants/{tenant}"
	s.api("GET "+tenant+"/packages", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.packages.Catalog(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), httpapi.PageRequest(r.URL.Query()))
		if e == nil {
			httpapi.Send(w, 200, v)
		}
		return e
	})
	s.api("GET "+tenant+"/packages/{key}/{version}", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.packages.Inspect(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), r.PathValue("key"), r.PathValue("version"))
		return httpapi.Reply(w, 200, v, e)
	})
	s.api("POST "+tenant+"/package-plans", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		var in apppackages.Selection
		if e := httpapi.Decode(w, r, &in); e != nil {
			return e
		}
		v, e := s.packages.Plan(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), in)
		return httpapi.Reply(w, 200, v, e)
	})
	s.api("GET "+tenant+"/package-stages", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.packages.Stages(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), httpapi.PageRequest(r.URL.Query()))
		if e == nil {
			httpapi.Send(w, 200, v)
		}
		return e
	})
	s.api("POST "+tenant+"/package-stages", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		var in apppackages.StageInput
		if e := httpapi.Decode(w, r, &in); e != nil {
			return e
		}
		v, e := s.packages.Stage(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), in)
		return httpapi.Reply(w, 202, v, e)
	})
	s.api("GET "+tenant+"/package-stages/{stage}", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.packages.Status(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), r.PathValue("stage"))
		return httpapi.Reply(w, 200, v, e)
	})
	s.api("POST "+tenant+"/package-stages/{stage}/{action}", s.packageAction)
	s.api("GET "+tenant+"/package-stages/{stage}/retry-plan", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.packages.ReviewRetry(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), r.PathValue("stage"))
		return httpapi.Reply(w, 200, v, e)
	})
	s.api("GET "+tenant+"/my-apps", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.packages.MyApps(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), httpapi.PageRequest(r.URL.Query()))
		if e == nil {
			httpapi.Send(w, 200, v)
		}
		return e
	})
	s.api("GET /v1/releases/{release}/source", s.packageSource)
	s.api("GET /v1/apps/{app}/source-info", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.delivery.SourceForApp(r.Context(), httpapi.Actor(r, p), r.PathValue("app"))
		return httpapi.Reply(w, 200, registry.Row{"release_id": v}, e)
	})
	s.api("GET /v1/apps/{app}/local-config", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.delivery.WorkspaceConfiguration(r.Context(), httpapi.Actor(r, p), r.PathValue("app"))
		if e == nil {
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Content-Disposition", "attachment; filename=lastapp.local.json")
		}
		return httpapi.Reply(w, 200, v, e)
	})
}

func (s *Server) packageAction(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	ctx, a, tenant, id := r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), r.PathValue("stage")
	var v apppackages.Stage
	var e error
	switch r.PathValue("action") {
	case "preview":
		v, e = s.packages.Preview(ctx, a, tenant, id)
	case "activate":
		v, e = s.packages.Activate(ctx, a, tenant, id)
	case "retry":
		v, e = s.packages.Retry(ctx, a, tenant, id, httpapi.IfMatch(r), r.URL.Query().Get("base_hash"))
	case "discard":
		if e = s.packages.Discard(ctx, a, tenant, id); e != nil {
			return e
		}
		httpapi.Send(w, 200, registry.Row{"id": id})
		return nil
	default:
		return registry.Missing()
	}
	return httpapi.Reply(w, 200, v, e)
}

func (s *Server) packageSource(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	f, e := s.delivery.ReleaseSource(r.Context(), httpapi.Actor(r, p), r.PathValue("release"))
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=lastapp-source.zip")
	http.ServeContent(w, r, "lastapp-source.zip", st.ModTime(), f)
	return nil
}

func (s *Server) PrunePackageArtifacts(ctx context.Context) (int, error) {
	return s.delivery.PruneArtifacts(ctx)
}

func (s *Server) RetirePackage(ctx context.Context, key, version string) error {
	return s.packages.Retire(ctx, key, version)
}
