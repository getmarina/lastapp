// Package agent serves delegated agent access: the OAuth authorization server
// for MCP clients (registration, consent, code/refresh exchange, revocation),
// the console agent-access endpoints and the management MCP server whose
// typed tools call the same validated operations as the console.
package agent

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/apppackages"
	"github.com/aantich/lastapp/internal/config"
	"github.com/aantich/lastapp/internal/databases"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entities"
	"github.com/aantich/lastapp/internal/httpapi"
	"github.com/aantich/lastapp/internal/inventory"
	"github.com/aantich/lastapp/internal/membership"
	"github.com/aantich/lastapp/internal/registry"
	"github.com/aantich/lastapp/internal/roles"
)

// Server is the agent layer, wired by control in Open. The slot channels
// bound concurrent MCP calls (Slots), anonymous OAuth registration and
// authorization (BootstrapSlots), token exchange/revocation (TokenSlots) and
// MCP bearer validation (ValidationSlots); size them with the constants below.
type Server struct {
	*access.Checker
	DB        *sql.DB
	Config    *config.Config
	Inventory *inventory.Inventory
	Members   *membership.Members
	Authority *roles.Authority
	Storage   *databases.Storage
	Authoring *entities.Authoring
	Delivery  *delivery.Service
	Packages  *apppackages.Service
	// Limiter is the per-principal request limiter shared with console sessions.
	Limiter *httpapi.Limiter
	Proxies *httpapi.TrustedProxies

	Slots             chan struct{}
	BootstrapSlots    chan struct{}
	TokenSlots        chan struct{}
	ValidationSlots   chan struct{}
	SourceSlots       chan struct{}
	bootstrapLimiter  httpapi.Limiter
	validationLimiter httpapi.Limiter
}

// MCPURL is the MCP resource URI of a deployment: the audience every agent
// grant, authorization code and token is bound to.
func MCPURL(publicURL string) string { return publicURL + "/mcp" }

func (s *Server) mcpURL() string { return MCPURL(s.Config.PublicURL) }

// ValidateClients checks the pre-registered agent clients of a configuration.
func ValidateClients(clients []config.AgentClient) error {
	seenClients := map[string]bool{}
	for _, client := range clients {
		if client.ID == "" || len(client.ID) > 500 || client.Name == "" || len(client.Name) > 160 || seenClients[client.ID] || len(client.Redirects) == 0 || len(client.Redirects) > agentMaxClientRedirects {
			return fmt.Errorf("invalid or duplicate agent client registration")
		}
		seenClients[client.ID] = true
		for _, redirect := range client.Redirects {
			if !validAgentRedirect(redirect) {
				return fmt.Errorf("invalid agent client redirect")
			}
		}
	}
	return nil
}

// Routes registers the OAuth authorization server, the console agent-access
// endpoints (wrapped by the console session supplied by control) and /mcp.
func (s *Server) Routes(mux *http.ServeMux, console httpapi.Session) {
	mux.HandleFunc("GET /mcp/sources/{release}", s.sourceDownload)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		httpapi.Send(w, 200, registry.Row{"resource": s.mcpURL(), "authorization_servers": []string{s.Config.PublicURL}, "scopes_supported": ScopeCodes(), "bearer_methods_supported": []string{"header"}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		httpapi.Send(w, 200, registry.Row{"issuer": s.Config.PublicURL, "authorization_endpoint": s.Config.PublicURL + "/oauth/authorize", "token_endpoint": s.Config.PublicURL + "/oauth/token", "registration_endpoint": s.Config.PublicURL + "/oauth/register", "revocation_endpoint": s.Config.PublicURL + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": ScopeCodes(), "client_id_metadata_document_supported": true, "authorization_response_iss_parameter_supported": true})
	})
	mux.HandleFunc("POST /oauth/register", s.RegisterClient)
	mux.HandleFunc("GET /oauth/authorize", s.agentAuthorize)
	mux.HandleFunc("POST /oauth/token", s.Token)
	mux.HandleFunc("POST /oauth/revoke", s.agentRevokeToken)
	mux.HandleFunc("GET /v1/agent-access/config", console.Handle(func(w http.ResponseWriter, r *http.Request, p access.Principal) error {
		httpapi.Send(w, 200, registry.Row{"mcp_url": s.mcpURL(), "permissions": agentPermissionCatalog(), "presets": agentPresets, "default_permissions": agentDefaultPermissions, "default_limits": access.AgentLimits{TargetIDs: []string{}, MaxDatabases: 1}, "default_days": agentGrantDays, "max_days": agentMaxGrantDays, "day_choices": agentGrantDayChoices, "max_connections": agentMaxConnections, "max_databases": agentMaxDatabases, "max_targets": agentMaxTargets, "access_token_seconds": int(agentAccessLifetime.Seconds()), "docs": agentGuides})
		return nil
	}))
	mux.HandleFunc("GET /v1/agent-access", console.Handle(s.agentGrantList))
	mux.HandleFunc("PATCH /v1/agent-access/{grant}", console.Handle(s.agentGrantUpdate))
	mux.HandleFunc("GET /v1/agent-access/requests/{request}", console.Handle(s.agentConsentInfo))
	mux.HandleFunc("POST /v1/agent-access/requests/{request}", console.Handle(s.agentConsent))
	mux.Handle("/mcp", s.MCPHandler())
}

const (
	agentAccessLifetime     = 15 * time.Minute
	agentGrantDays          = 7
	agentMaxGrantDays       = 30
	agentCodeLifetime       = 5 * time.Minute
	MaxResponse             = 2 << 20
	agentMaxDatabases       = 20
	agentMaxTargets         = 32
	agentMaxConnections     = 100
	MaxConcurrent           = 16
	agentMaxTokenRotations  = 4096
	agentMaxSyntheticCases  = 10
	MaxClients              = 1000
	BootstrapConcurrent     = 4
	TokenConcurrent         = 4
	ValidationConcurrent    = 8
	SourceConcurrent        = 4
	agentMaxClientRedirects = 16
	MaxClientMetadata       = 65536
)

// Principal authenticates an MCP bearer token: its token-bound grant,
// user and live delegation check.
func (s *Server) Principal(r *http.Request) (access.Principal, error) {
	var p access.Principal
	raw := r.Header.Get("Authorization")
	if len(raw) < 8 || len(raw) > 512 || raw[:7] != "Bearer " {
		return p, registry.Problem(401, "unauthenticated", "An MCP access token is required.")
	}
	// Load the token-bound grant and its permissions in one statement/snapshot.
	// A later grant edit fails checkAgent; it cannot upgrade this access token.
	var e error
	p.Agent, e = access.ScanAgentGrant(r.Context(), s.DB, `SELECT `+access.AgentGrantColumns+`,u.display_name,COALESCE(u.email,'') FROM agent_token_sessions se JOIN agent_grants g ON g.id=se.grant_id AND g.revision=se.grant_revision JOIN users u ON u.id=g.user_id AND u.state='active' WHERE se.access_hash=? AND se.state='active' AND se.access_expires_at>UTC_TIMESTAMP(6) AND g.state='active' AND g.expires_at>UTC_TIMESTAMP(6)`, []any{registry.TokenHash(raw[7:])}, &p.Name, &p.Email)
	if p.Agent == nil {
		return p, registry.Problem(401, "unauthenticated", "Invalid or expired MCP access token.")
	}
	if e != nil {
		return p, e
	}
	p.ID = p.Agent.User
	p.Platform, e = access.IsPlatform(r.Context(), s.DB, p.ID)
	if e != nil {
		return p, e
	}
	return p, s.CheckAgent(r.Context(), s.DB, p, access.Resource{Tenant: p.Agent.Tenant}, false)
}

// Admission happens before token/database work. Anonymous discovery/metadata
// fetching cannot consume authenticated MCP slots or all registry connections.
func (s *Server) admitAgent(w http.ResponseWriter, r *http.Request, limiter *httpapi.Limiter, slots chan struct{}, prefix string, burst int, refill float64) bool {
	if !limiter.AllowRate(prefix+":"+s.Proxies.ClientIP(r), time.Now(), float64(burst), refill) {
		httpapi.WriteError(w, r, httpapi.RateLimited(w))
		return false
	}
	select {
	case slots <- struct{}{}:
		return true
	default:
		httpapi.WriteError(w, r, httpapi.RateLimited(w))
		return false
	}
}
