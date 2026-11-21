package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/config"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/httpapi"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

func TestAgentToolContracts(t *testing.T) {
	s := &Server{}
	tools := s.Tools()
	if len(tools) != 18 {
		t.Fatal("unexpected tool surface", len(tools))
	}
	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			schema, e := InputSchema(tool.Input)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = schema.Resolve(nil); e != nil {
				t.Fatal(e)
			}
			if len(tool.Description) < 100 {
				t.Fatal("missing agent guidance")
			}
		})
	}
	schema, e := InputSchema(reflect.TypeFor[EntityInput]())
	if e != nil {
		t.Fatal(e)
	}
	resolved, e := schema.Resolve(nil)
	if e != nil {
		t.Fatal(e)
	}
	var instance any
	encoded, _ := json.Marshal(EntityInput{Definition: &entity.Templates()[2], RequestKey: registry.NewID()})
	if e = json.Unmarshal(encoded, &instance); e != nil {
		t.Fatal(e)
	}
	if e = resolved.Validate(instance); e != nil {
		t.Fatal("default/calculated expression pointers are not representable", e)
	}
	for _, raw := range []string{"http://evil.example/callback", "https://u:p@example.com/cb", "file:///tmp/cb", "https://example.com/cb#fragment"} {
		if validAgentRedirect(raw) {
			t.Fatal("unsafe redirect", raw)
		}
	}
	if !agentRedirectMatches("http://127.0.0.1:8790/callback", "http://127.0.0.1:12345/callback") || agentRedirectMatches("http://127.0.0.1:8790/callback", "http://localhost:12345/callback") {
		t.Fatal("loopback redirect mismatch")
	}
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "169.254.169.254", "100.64.0.2", "::ffff:127.0.0.1", "2001:db8::1", "64:ff9b::7f00:1"} {
		if publicAgentIP(ip) {
			t.Fatal("SSRF address accepted", ip)
		}
	}
}

func TestAgentPreauthenticationAdmission(t *testing.T) {
	// No database: these denied requests must never reach token lookup.
	s := &Server{Config: &config.Config{}, Proxies: &httpapi.TrustedProxies{}, ValidationSlots: make(chan struct{}, ValidationConcurrent)}
	r := httptest.NewRequest("POST", "http://localhost/mcp", nil)
	r.Header.Set("Authorization", "Bearer unknown")
	for i := 0; i < ValidationConcurrent; i++ {
		s.ValidationSlots <- struct{}{}
	}
	w := httptest.NewRecorder()
	s.MCPHandler().ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatal("validation concurrency admission was not enforced")
	}
	for i := 0; i < ValidationConcurrent; i++ {
		<-s.ValidationSlots
	}
	for s.validationLimiter.AllowRate("mcp-auth:"+s.Proxies.ClientIP(r), time.Now(), 60, 0) {
	}
	w = httptest.NewRecorder()
	s.MCPHandler().ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatal("invalid bearer rate admission was not enforced before database work")
	}
}

// TestAgentOperationResponseLimit pins the MCP response bound on operation
// results: an over-limit read fails without content, and an over-limit result
// of a committed mutation still reports the commit and its compact resource so
// the agent does not retry a mutation that already happened.
func TestAgentOperationResponseLimit(t *testing.T) {
	large := registry.Row{"payload": strings.Repeat("x", MaxResponse)}
	isLimit := func(e error) bool {
		var ae *registry.APIError
		return errors.As(e, &ae) && ae.Code == "response_limit" && ae.Status == 413
	}
	if v, e := agentSized(registry.Row{"id": "small"}, nil); e != nil || v.(registry.Row)["id"] != "small" {
		t.Fatal("in-limit result was not passed through", v, e)
	}
	failed := errors.New("operation failed")
	if v, e := agentSized(large, failed); v != nil || !errors.Is(e, failed) {
		t.Fatal("operation error was not returned unchanged", v, e)
	}
	// A read has no agent receipt: the over-limit result is an error with no data.
	v, e := agentSized(large, nil)
	if v != nil || !isLimit(e) {
		t.Fatal("over-limit read result was returned", e)
	}
	ref := registry.Row{"kind": "app", "id": registry.NewID()}
	call := &access.AgentCall{Operation: "lastapp_create", Key: registry.NewID(), ResultRef: ref}
	ctx := context.WithValue(context.Background(), access.AgentContextKey{}, call)
	// The mutation did not commit: the agent sees the limit error and may retry.
	data, limit := agentSized(large, nil)
	if v, e := agentWriteResult(ctx, data, limit); v != nil || !isLimit(e) {
		t.Fatal("uncommitted over-limit mutation reported success", v, e)
	}
	// The mutation committed: the reply is the compact committed outcome.
	call.Committed = true
	v, e = agentWriteResult(ctx, data, limit)
	if e != nil {
		t.Fatal("committed over-limit mutation returned an error an agent would retry", e)
	}
	out := v.(registry.Row)
	if out["committed"] != true || !reflect.DeepEqual(out["resource"], ref) || out["response_error"].(registry.Row)["code"] != "response_limit" {
		t.Fatal("committed over-limit mutation lost its committed resource", out)
	}
	if e = agentFits(out); e != nil {
		t.Fatal("compact committed reply exceeds the response bound", e)
	}
	// Other errors after a commit pass through unchanged.
	if _, e = agentWriteResult(ctx, nil, failed); !errors.Is(e, failed) {
		t.Fatal("non-limit error was rewritten", e)
	}
	// A replayed request reports the stored receipt and marks the call committed.
	replayed := &access.AgentCall{Operation: "lastapp_create", Key: registry.NewID(), ResultRef: ref}
	v, e = agentWriteResult(context.WithValue(context.Background(), access.AgentContextKey{}, replayed), nil, op.ErrAgentReplay)
	if e != nil || v.(registry.Row)["replayed"] != true || v.(registry.Row)["committed"] != true || !replayed.Committed {
		t.Fatal("replayed mutation was not reported as committed", v, e)
	}
}

// TestAgentToolTable pins the catalog invariants the MCP server relies on:
// unique lastapp_ names, an adapter for each tool, schema input types that are
// structs, and grant permissions that exist in the published catalog.
func TestAgentToolTable(t *testing.T) {
	s := &Server{}
	codes := ScopeCodes()
	seen := map[string]bool{}
	for _, tool := range s.Tools() {
		if !strings.HasPrefix(tool.Name, "lastapp_") || seen[tool.Name] {
			t.Fatal("tool names must be unique and prefixed", tool.Name)
		}
		seen[tool.Name] = true
		if tool.Run == nil || tool.Input == nil || tool.Input.Kind() != reflect.Struct {
			t.Fatal("tool lacks an adapter or a struct input", tool.Name)
		}
		for _, p := range tool.Permissions {
			if !slices.Contains(codes, p) {
				t.Fatal("tool requires an unpublished permission", tool.Name, p)
			}
		}
		if tool.Read && !slices.Equal(tool.Permissions, []string{"read"}) && tool.Name != "lastapp_entity_check" {
			t.Fatal("read-only tool requires a write permission", tool.Name)
		}
	}
	// Without any grant permission only tools with no required permission are
	// listed, and each of those still needs a matching write permission.
	for _, tool := range s.Tools() {
		if ToolVisible(tool, &access.AgentGrant{}) {
			t.Fatal("tool visible to a grant without permissions", tool.Name)
		}
	}
	read := &access.AgentGrant{Permissions: []string{"read"}}
	for _, name := range []string{"lastapp_context", "lastapp_list", "lastapp_get", "lastapp_describe", "lastapp_workspace", "lastapp_release", "lastapp_create"} {
		i := slices.IndexFunc(s.Tools(), func(v ToolSpec) bool { return v.Name == name })
		want := name != "lastapp_workspace" && name != "lastapp_release" && name != "lastapp_create"
		if ToolVisible(s.Tools()[i], read) != want {
			t.Fatal("read-only grant visibility changed", name)
		}
	}
}

// TestAgentPermissionCatalog pins the delegation catalog: every permission is
// grantable at some scope kind, presets and defaults name catalog codes, and
// requested OAuth scopes normalize to sorted, unique catalog codes.
func TestAgentPermissionCatalog(t *testing.T) {
	codes := ScopeCodes()
	for _, p := range agentPermissionCatalog() {
		if len(p["scope_kinds"].([]string)) == 0 {
			t.Fatal("permission without a scope kind", p["code"])
		}
	}
	for _, preset := range agentPresets {
		for _, code := range preset["permissions"].([]string) {
			if !slices.Contains(codes, code) {
				t.Fatal("preset names an unknown permission", code)
			}
		}
	}
	for _, preset := range agentPresets {
		if registry.Str(preset["description"]) == "" {
			t.Fatal("preset without a description", preset["label"])
		}
	}
	if !slices.Contains(agentGrantDayChoices, agentGrantDays) {
		t.Fatal("the default expiry is not one of the choices")
	}
	for _, days := range agentGrantDayChoices {
		if days < 1 || days > agentMaxGrantDays {
			t.Fatal("expiry choice outside 1..max_days", days)
		}
	}
	for _, code := range agentDefaultPermissions {
		if !slices.Contains(codes, code) {
			t.Fatal("default names an unknown permission", code)
		}
	}
	got, e := agentScopeNames("read resources.write read")
	if e != nil || !slices.Equal(got, []string{"read", "resources.write"}) {
		t.Fatal("scopes were not normalized", got, e)
	}
	if all, e := agentScopeNames(""); e != nil || len(all) != len(codes) {
		t.Fatal("an empty scope must offer every capability at consent", all, e)
	}
	if _, e := agentScopeNames("read admin"); e == nil {
		t.Fatal("unknown scope accepted")
	}
}

// TestAgentClientValidation pins the configured-client checks run by Open.
func TestAgentClientValidation(t *testing.T) {
	ok := config.AgentClient{ID: "client", Name: "Client", Redirects: []string{"http://127.0.0.1:8790/callback"}}
	if e := ValidateClients([]config.AgentClient{ok}); e != nil {
		t.Fatal(e)
	}
	for name, clients := range map[string][]config.AgentClient{
		"duplicate":    {ok, ok},
		"no redirects": {{ID: "c", Name: "C"}},
		"unsafe":       {{ID: "c", Name: "C", Redirects: []string{"http://evil.example/cb"}}},
		"unnamed":      {{ID: "c", Redirects: ok.Redirects}},
	} {
		if ValidateClients(clients) == nil {
			t.Fatal("invalid client configuration accepted:", name)
		}
	}
	if MCPURL("https://lastapp.example") != "https://lastapp.example/mcp" {
		t.Fatal("MCP resource URI changed")
	}
}
