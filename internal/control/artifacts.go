package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/httpapi"
	"github.com/aantich/lastapp/internal/registry"
)

func (s *Server) releaseRoutes() {
	s.api("POST /v1/apps/{app}/upload-tickets", s.artifactTicket)
	s.mux.HandleFunc("POST /artifact/v1/upload", s.artifactUpload)
	s.api("GET /v1/apps/{app}/builds", s.buildList)
	s.api("GET /v1/apps/{app}/releases", s.releaseList)
	s.api("GET /v1/apps/{app}/deployment", s.appDeployment)
	s.api("POST /v1/apps/{app}/releases", s.releaseCreate)
	s.api("GET /v1/releases/{release}", s.releaseGet)
	s.api("POST /v1/releases/{release}/preview", s.releasePreview)
	s.api("POST /v1/releases/{release}/deploy", s.releaseDeploy)
	s.api("GET /v1/deployments/{deployment}", s.deploymentGet)
	s.api("POST /v1/deployments/{deployment}/retry", s.deploymentRetry)
	s.api("GET /v1/apps/{app}/starter", s.appStarter)
	s.api("GET /v1/apps/{app}/starter.zip", s.appStarterArchive)
}

func (s *Server) artifactUpload(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	run := func() error {
		if !s.limiter.AllowRate("upload:"+s.trustedProxies.ClientIP(r), time.Now(), 20, 4) {
			return httpapi.RateLimited(w)
		}
		select {
		case s.uploadSlots <- struct{}{}:
			defer func() { <-s.uploadSlots }()
		default:
			return httpapi.RateLimited(w)
		}
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(raw) != 43 {
			return registry.Problem(401, "invalid_upload", "Upload ticket required.")
		}
		upload, e := s.delivery.AuthorizeUpload(ctx, raw)
		if e != nil {
			return e
		}
		if origin := r.Header.Get("Origin"); origin != "" && !httpapi.AllowedOrigin(&s.Config, origin) {
			return registry.Problem(403, "invalid_origin", "Upload is restricted to trusted authoring clients.")
		}
		var in struct {
			Files  []artifact.File `json:"files"`
			Entry  string          `json:"entrypoint"`
			Source []byte          `json:"source_zip,omitempty"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, artifact.MaxBody+((artifact.SourceLimit+2)/3)*4+128))
		d.DisallowUnknownFields()
		if d.Decode(&in) != nil {
			return registry.Bad("Invalid or oversized static build upload.")
		}
		var more any
		if !errors.Is(d.Decode(&more), io.EOF) {
			return registry.Bad("One upload object required.")
		}
		v, created, e := s.delivery.StoreBuild(ctx, upload, in.Files, in.Entry, r.Header.Get("X-Request-ID"), in.Source)
		if e != nil {
			return e
		}
		if created {
			httpapi.Send(w, 201, v)
		} else {
			httpapi.Send(w, 200, v)
		}
		return nil
	}
	if e := run(); e != nil {
		httpapi.WriteError(w, r, e)
	}
}

// App hosts are selected before the console router. A hostile App can never
// serve active content on the console origin or another App's origin.
func (s *Server) serveAppHost(w http.ResponseWriter, r *http.Request) bool {
	suffix := delivery.HostNamespace(delivery.HostingBase(&s.Config))
	if suffix == "" {
		return false
	}
	// The whole App hostname namespace is static-only, regardless of the port
	// spelling used by a browser or reverse proxy. Never fall back to management.
	host := strings.TrimSuffix(strings.ToLower((&url.URL{Host: r.Host}).Hostname()), ".")
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	app := strings.TrimSuffix(host, suffix)
	if registry.ID(app) != nil {
		http.NotFound(w, r)
		return true
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' "+s.Config.PublicURL+"; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	development, release, e := s.delivery.HostedApp(ctx, app)
	if e != nil {
		http.NotFound(w, r)
		return true
	}
	if r.URL.Path == "/_lastapp/config.json" {
		httpapi.Send(w, 200, registry.Row{"apiOrigin": s.Config.PublicURL, "appID": app, "environment": map[bool]string{true: "development", false: "production"}[development]})
		return true
	}
	if release == nil {
		http.NotFound(w, r)
		return true
	}
	prefix := "/_lastapp/builds/"
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		build, entry, e := s.delivery.ReleaseEntry(ctx, *release)
		if e != nil {
			http.NotFound(w, r)
			return true
		}
		http.Redirect(w, r, prefix+build+"/"+entry, 302)
		return true
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, prefix), "/", 2)
	if !strings.HasPrefix(r.URL.Path, prefix) || len(parts) != 2 || registry.ID(parts[0]) != nil || !artifact.ValidPath(parts[1]) {
		http.NotFound(w, r)
		return true
	}
	file, size, e := s.delivery.HostedFile(ctx, app, parts[0], parts[1])
	if e != nil {
		http.NotFound(w, r)
		return true
	}
	defer file.Close()
	contentType := mime.TypeByExtension(path.Ext(parts[1]))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Method == "GET" {
		_, _ = io.Copy(w, file)
	}
	return true
}

func (s *Server) artifactTicket(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	v, e := s.delivery.UploadTicket(r.Context(), httpapi.Actor(r, p), r.PathValue("app"))
	return httpapi.Reply(w, 201, v, e)
}

func (s *Server) buildList(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	v, e := s.delivery.Builds(r.Context(), httpapi.Actor(r, p), r.PathValue("app"), httpapi.PageRequest(r.URL.Query()))
	return httpapi.Reply(w, 200, v, e)
}
