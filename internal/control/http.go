package control

import (
	"context"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/authz"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/httpapi"
	"github.com/aantich/lastapp/internal/membership"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/roles"
	admin "github.com/aantich/lastapp/web/admin"
)

// console is the console-session wrapper of every authenticated console route.
func (s *Server) console() httpapi.Session {
	return httpapi.Session{Principal: s.sessions.Principal, Limit: func(w http.ResponseWriter, p access.Principal, r *http.Request) error {
		return s.limiter.LimitRequest(w, r, s.Checker, s.DB, p)
	}, CSRF: s.sessions.CSRF}
}

func (s *Server) api(pattern string, fn httpapi.Endpoint) {
	s.mux.HandleFunc(pattern, s.console().Handle(fn))
}

func (s *Server) Handler() http.Handler {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { httpapi.Send(w, 200, registry.Row{"status": "ok"}) })
	s.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if s.DB.PingContext(ctx) != nil {
			httpapi.Send(w, 503, registry.Row{"status": "unavailable"})
			return
		}
		httpapi.Send(w, 200, registry.Row{"status": "ready"})
	})
	s.mux.HandleFunc("GET /v1/config", func(w http.ResponseWriter, r *http.Request) {
		httpapi.Send(w, 200, registry.Row{"name": "Lastapp", "version": "0.1.0-dev", "development": s.Config.Development, "oidc_enabled": s.Config.OIDCIssuer != "", "login_url": "/auth/login", "csrf_cookie": httpapi.CookieName(s.Config.PublicURL, httpapi.CSRFCookie), "development_policy": delivery.Policy(), "password_policy": membership.PublicPasswordPolicy(), "role_catalog": authz.Roles()})
	})
	s.mux.HandleFunc("GET /auth/login", s.sessions.Login)
	s.mux.HandleFunc("GET /auth/callback", s.sessions.Callback)
	s.mux.HandleFunc("POST /auth/login/password", s.sessions.PasswordLogin)
	s.api("POST /auth/password", s.sessions.ChangePassword)
	s.api("POST /auth/logout", s.sessions.Logout)
	s.api("GET /v1/me", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		ts, e := s.myTenants(r, p)
		if e != nil {
			return e
		}
		httpapi.Send(w, 200, registry.Row{"id": p.ID, "display_name": p.Name, "email": p.Email, "platform_admin": p.Platform, "auth_method": p.AuthMethod, "password_change_required": p.PasswordChangeRequired, "tenants": ts, "pagination": httpapi.PageInfo(r.Context())})
		return nil
	})
	s.api("GET /v1/my-tenants", func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		v, e := s.myTenants(r, p)
		if e == nil {
			httpapi.SendPage(w, r, v)
		}
		return e
	})
	s.entityRoutes()
	s.agent.Routes(s.mux, s.console())
	s.workspaceRoutes()
	s.releaseRoutes()
	s.packageRoutes()
	s.api("GET /v1/roles/catalog", s.roleCatalog)
	s.api("GET /v1/tenants/{tenant}/my-role-assignments", s.myRoles)
	s.api("GET /v1/tenants/{tenant}/capabilities", s.capabilities)
	s.api("GET /admin/v1/overview", s.overview)
	s.api("GET /admin/v1/users", s.users)
	s.api("POST /admin/v1/users", s.createLocalUser)
	s.api("PATCH /admin/v1/users/{id}", s.updateUser)
	s.api("GET /admin/v1/plans", s.plans)
	s.api("POST /admin/v1/plans", s.createPlan)
	s.api("PATCH /admin/v1/plans/{id}", s.updatePlan)
	s.api("GET /admin/v1/tenants", s.tenants)
	s.api("POST /admin/v1/tenants", s.createTenant)
	s.api("PATCH /admin/v1/tenants/{id}", s.updateTenant)
	s.api("GET /v1/tenants/{tenant}", s.getTenant)
	s.api("PATCH /v1/tenants/{tenant}", s.updateTenantProfile)
	s.api("GET /v1/tenants/{tenant}/overview", s.tenantOverview)
	s.api("GET /admin/v1/tenants/{tenant}/administrators", s.tenantAdministrators)
	s.api("POST /admin/v1/tenants/{tenant}/administrators", s.addTenantAdministrator)
	s.api("GET /admin/v1/database-targets", s.targets)
	s.api("POST /admin/v1/database-targets", s.createTarget)
	s.api("PATCH /admin/v1/database-targets/{id}", s.updateTarget)
	s.api("GET /v1/tenants/{tenant}/database-targets", s.availableTargets)
	s.api("GET /v1/tenants/{tenant}/database-resources", s.databases)
	s.api("POST /v1/tenants/{tenant}/database-resources", s.createDatabase)
	s.api("GET /v1/tenants/{tenant}/database-resources/{id}", s.getDatabase)
	s.api("POST /v1/tenants/{tenant}/database-resources/{id}/retry", s.retryDatabase)
	s.api("PATCH /v1/tenants/{tenant}/database-resources/{id}", s.updateDatabase)
	s.api("GET /v1/tenants/{tenant}/spaces", s.spaces)
	s.api("GET /v1/tenants/{tenant}/spaces/{space}", s.getSpace)
	s.api("POST /v1/tenants/{tenant}/spaces", s.createSpace)
	s.api("PATCH /v1/tenants/{tenant}/spaces/{id}", s.updateSpace)
	s.api("GET /v1/tenants/{tenant}/apps", s.apps)
	s.api("GET /v1/tenants/{tenant}/apps/{app}", s.getApp)
	s.api("POST /v1/tenants/{tenant}/spaces/{space}/apps", s.createApp)
	s.api("PATCH /v1/tenants/{tenant}/apps/{id}", s.updateApp)
	s.api("GET /v1/tenants/{tenant}/microapps", s.microapps)
	s.api("POST /v1/tenants/{tenant}/apps/{app}/microapps", s.createMicroapp)
	s.api("PATCH /v1/tenants/{tenant}/microapps/{id}", s.updateMicroapp)
	s.api("GET /v1/tenants/{tenant}/memberships", s.memberships)
	s.api("POST /v1/tenants/{tenant}/memberships", s.addMembership)
	s.api("PATCH /v1/tenants/{tenant}/memberships/{user}", s.updateMembership)
	s.api("GET /v1/tenants/{tenant}/teams", s.groups)
	s.api("GET /v1/tenants/{tenant}/teams/{id}", s.getGroup)
	s.api("POST /v1/tenants/{tenant}/teams", s.createGroup)
	s.api("PATCH /v1/tenants/{tenant}/teams/{id}", s.updateGroup)
	s.api("GET /v1/tenants/{tenant}/teams/{id}/members", s.groupMembers)
	s.api("POST /v1/tenants/{tenant}/teams/{id}/members", s.addGroupMember)
	s.api("DELETE /v1/tenants/{tenant}/teams/{id}/members/{user}", s.removeGroupMember)
	for _, path := range []string{"/v1/tenants/{tenant}", "/v1/tenants/{tenant}/teams/{team}", "/v1/tenants/{tenant}/spaces/{space}", "/v1/tenants/{tenant}/apps/{app}"} {
		s.api("GET "+path+"/recipients", s.recipientOptions)
		s.api("GET "+path+"/roles", s.roles)
		if strings.Contains(path, "{space}") || strings.Contains(path, "{app}") {
			s.api("POST "+path+"/roles", s.createRole)
			s.api("PATCH "+path+"/roles/{id}", s.updateRole)
		}
		s.api("GET "+path+"/role-assignments", s.grants)
		s.api("POST "+path+"/role-assignments", s.createGrant)
		s.api("DELETE "+path+"/role-assignments/{id}", s.deleteGrant)
	}
	s.api("GET /admin/v1/role-assignments", s.grants)
	s.api("POST /admin/v1/role-assignments", s.createGrant)
	s.api("DELETE /admin/v1/role-assignments/{id}", s.deleteGrant)
	s.api("PATCH /v1/tenants/{tenant}/teams/{id}/members/{user}", s.updateGroupMember)
	s.api("GET /v1/tenants/{tenant}/audit-events", s.audits)
	s.api("GET /admin/v1/audit-events", s.audits)
	s.api("GET /v1/tenants/{tenant}/audit-kinds", s.auditKinds)
	s.api("GET /admin/v1/audit-kinds", s.auditKinds)
	assets, _ := fs.Sub(admin.Files, "dist")
	files := http.FileServer(http.FS(assets))
	s.mux.HandleFunc("GET /console/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/console/")
		if name == "/console" || name == "." {
			name = "index.html"
		}
		if _, e := fs.Stat(assets, name); e != nil {
			if strings.Contains(path.Base(name), ".") {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		if name == "index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			b, err := fs.ReadFile(assets, name)
			if err != nil {
				http.Error(w, "Build the admin UI before compiling Lastapp (make build).", 503)
				return
			}
			_, _ = w.Write(b) // client disconnects are not actionable
			return
		}
		clone := r.Clone(r.Context())
		clone.URL.Path = "/" + name
		files.ServeHTTP(w, clone)
	})
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/console/", 302) })
	return httpapi.Observe(s.Log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.serveAppHost(w, r) {
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		s.mux.ServeHTTP(w, r)
	}))
}

// TenantRef is a Tenant the principal belongs to.
type TenantRef struct {
	Name  string `json:"display_name"`
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	State string `json:"state"`
}

func (v *TenantRef) ScanFields() []any { return []any{&v.Name, &v.ID, &v.Slug, &v.State} }

// myTenants pages the Tenants where the principal has an active membership.
// It is the one query behind both /v1/me and /v1/my-tenants.
func (s *Server) myTenants(r *http.Request, p access.Principal) ([]TenantRef, error) {
	return httpapi.PageRows[TenantRef](r.Context(), s.DB, r, "SELECT t.display_name,t.id,t.slug,t.state FROM tenant_memberships m JOIN tenants t ON t.id=m.tenant_id WHERE m.user_id=? AND m.state='active' ORDER BY t.display_name", p.ID)
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	tenant := r.PathValue("tenant")
	if !p.Platform {
		if e := s.Member(r.Context(), s.DB, p, tenant, false); e != nil {
			return e
		}
	}
	scopes, e := s.EffectiveScopes(r.Context(), s.DB, p, tenant)
	if e != nil {
		return e
	}
	out := registry.Row{}
	for code, sc := range scopes {
		out[code] = registry.Row{"all": sc.All, "available": sc.All || len(sc.Spaces)+len(sc.Apps)+len(sc.Teams) > 0, "spaces": sc.Spaces, "apps": sc.Apps, "teams": sc.Teams}
	}
	// The account's fixed control roles here, broadest first: the console
	// names the first under the account's name.
	held, e := s.ControlRoles(r.Context(), s.DB, p, tenant)
	if e != nil {
		return e
	}
	httpapi.Send(w, 200, registry.Row{"permissions": out, "control_roles": held})
	return nil
}

// stateCount is how many database resources are in a state.
type stateCount struct {
	Count int64  `json:"count"`
	State string `json:"state"`
}

func (v *stateCount) ScanFields() []any { return []any{&v.Count, &v.State} }

// recentDatabase is a recently created database resource on the platform
// overview.
type recentDatabase struct {
	Name        string  `json:"database_name"`
	ID          string  `json:"id"`
	LastMessage *string `json:"last_error_message"`
	State       string  `json:"state"`
	TenantID    string  `json:"tenant_id"`
	TenantName  string  `json:"tenant_name"`
}

func (v *recentDatabase) ScanFields() []any {
	return []any{&v.Name, &v.ID, &v.LastMessage, &v.State, &v.TenantID, &v.TenantName}
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	if e := s.Authorize(r.Context(), s.DB, p, "platform.admin", access.Resource{}, false); e != nil {
		return e
	}
	out := registry.Row{}
	for _, table := range []string{"tenants", "spaces", "apps", "database_resources", "users"} {
		var n int
		if e := s.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n); e != nil {
			return e
		}
		out[table] = n
	}
	status, e := registry.ScanAll[stateCount](r.Context(), s.DB, "SELECT COUNT(*),state FROM database_resources GROUP BY state")
	if e != nil {
		return e
	}
	out["database_states"] = status
	recent, e := registry.ScanAll[recentDatabase](r.Context(), s.DB, "SELECT d.database_name,d.id,d.last_error_message,d.state,d.tenant_id,t.display_name FROM database_resources d JOIN tenants t ON t.id=d.tenant_id ORDER BY d.created_at DESC LIMIT 6")
	if e != nil {
		return e
	}
	out["recent_databases"] = recent
	httpapi.Send(w, 200, out)
	return e
}

func result(w http.ResponseWriter, r *http.Request, s *Server, table, tenant, record string, status int) error {
	query := "SELECT id,revision FROM " + table + " WHERE id=?"
	args := []any{record}
	if tenant != "" {
		query += " AND tenant_id=?"
		args = append(args, tenant)
	}
	var v op.Ref
	e := registry.Lookup(r.Context(), s.DB, query, args, &v.ID, &v.Revision)
	if e == nil {
		w.Header().Set("ETag", strconv.FormatInt(v.Revision, 10))
		httpapi.Send(w, status, v)
	}
	return e
}

func (s *Server) audits(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	q := r.URL.Query()
	v, e := s.authority.Audits(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"), httpapi.PageRequest(q), roles.AuditFilter{Actor: q.Get("actor"), Kind: q.Get("kind"), Resource: q.Get("resource_id")})
	return httpapi.Reply(w, 200, v, e)
}

func (s *Server) auditKinds(w http.ResponseWriter, r *http.Request, p access.Principal) error {
	v, e := s.authority.AuditKinds(r.Context(), httpapi.Actor(r, p), r.PathValue("tenant"))
	return httpapi.Reply(w, 200, v, e)
}
