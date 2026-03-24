package api

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	proxyconfig "github.com/coachpo/cockpit-backend/internal/config"
	sdkconfig "github.com/coachpo/cockpit-backend/internal/config"
	sdkaccess "github.com/coachpo/cockpit-backend/sdk/access"
	"github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
	gin "github.com/gin-gonic/gin"
)

func newTestServer(t *testing.T, mutate func(*proxyconfig.Config), opts ...ServerOption) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port: 0,
	}
	if mutate != nil {
		mutate(cfg)
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()
	return NewServer(cfg, authManager, accessManager, nil, opts...)
}

type managementRouteCase struct {
	name        string
	method      string
	path        string
	body        string
	contentType string
}

func performManagementRequest(server *Server, route managementRouteCase, authHeader string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var bodyReader *strings.Reader
	if route.body != "" {
		bodyReader = strings.NewReader(route.body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(route.method, route.path, bodyReader)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if route.contentType != "" {
		req.Header.Set("Content-Type", route.contentType)
	}
	server.engine.ServeHTTP(rec, req)
	return rec
}

func managementBearerHeader(token string) string {
	return "Bearer " + token
}

func routeKey(method, path string) string {
	return method + " " + path
}

func mountedRouteSet(server *Server) map[string]struct{} {
	routes := make(map[string]struct{})
	for _, route := range server.engine.Routes() {
		routes[routeKey(route.Method, route.Path)] = struct{}{}
	}
	return routes
}

func sortedRouteKeys(routes map[string]struct{}) []string {
	keys := make([]string, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestManagementRetainedRouteRejectsMissingAuthorizationWhenPasswordConfigured(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "secret")

	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/api/runtime-settings",
	}, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected runtime-settings route status %d, got %d with body %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Missing management bearer token") {
		t.Fatalf("expected missing-token error body, got %s", rec.Body.String())
	}
}

func TestServerUsesConfiguredListenAddress(t *testing.T) {
	server := newTestServer(t, func(cfg *proxyconfig.Config) {
		cfg.Host = "0.0.0.0"
		cfg.Port = 8080
	})

	if server.server == nil {
		t.Fatal("expected underlying http server to be initialized")
	}
	if server.server.Addr != "0.0.0.0:8080" {
		t.Fatalf("expected listen address 0.0.0.0:8080, got %q", server.server.Addr)
	}
}

func TestCorsMiddlewareAllowsAllOrigins(t *testing.T) {
	server := newTestServer(t, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/runtime-settings", nil)
	req.Header.Set("Origin", "https://frontend.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	server.engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected CORS preflight status %d, got %d with body %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard allow-origin header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodGet) {
		t.Fatalf("expected allow-methods header to include %s, got %q", http.MethodGet, got)
	}
	if got := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers")); got != "*" && !strings.Contains(got, "authorization") {
		t.Fatalf("expected allow-headers to include authorization or *, got %q", got)
	}
}

func TestManagementRetainedRouteRejectsInvalidAuthorizationWhenPasswordConfigured(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "secret")

	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/api/runtime-settings",
	}, managementBearerHeader("wrong-secret"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected runtime-settings route status %d, got %d with body %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Invalid management bearer token") {
		t.Fatalf("expected invalid-token error body, got %s", rec.Body.String())
	}
	if got := rec.Header().Get("X-CPA-VERSION"); got != "" {
		t.Fatalf("expected no X-CPA-VERSION header, got %q", got)
	}
	if got := rec.Header().Get("X-CPA-COMMIT"); got != "" {
		t.Fatalf("expected no X-CPA-COMMIT header, got %q", got)
	}
	if got := rec.Header().Get("X-CPA-BUILD-DATE"); got != "" {
		t.Fatalf("expected no X-CPA-BUILD-DATE header, got %q", got)
	}
}

func TestManagementRetainedRouteAcceptsMatchingAuthorizationWhenPasswordConfigured(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "secret")

	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/api/runtime-settings",
	}, managementBearerHeader("secret"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime-settings route status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestOAuthCallbackRouteIsAccessibleWithoutAuthorization(t *testing.T) {
	server := newTestServer(t, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	server.engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected oauth callback route status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected oauth callback route to return html, got %q", got)
	}
}

func TestManagementRemovedRoutesAreNotMounted(t *testing.T) {
	server := newTestServer(t, nil)
	routes := mountedRouteSet(server)

	removedRoutes := []string{
		routeKey(http.MethodGet, "/api/ws-auth"),
		routeKey(http.MethodPut, "/api/ws-auth"),
		routeKey(http.MethodPatch, "/api/ws-auth"),
		routeKey(http.MethodGet, "/api/request-retry"),
		routeKey(http.MethodPut, "/api/request-retry"),
		routeKey(http.MethodPatch, "/api/request-retry"),
		routeKey(http.MethodGet, "/api/max-retry-interval"),
		routeKey(http.MethodPut, "/api/max-retry-interval"),
		routeKey(http.MethodPatch, "/api/max-retry-interval"),
		routeKey(http.MethodGet, "/api/routing/strategy"),
		routeKey(http.MethodPut, "/api/routing/strategy"),
		routeKey(http.MethodPatch, "/api/routing/strategy"),
		routeKey(http.MethodGet, "/api/quota-exceeded/switch-project"),
		routeKey(http.MethodPut, "/api/quota-exceeded/switch-project"),
		routeKey(http.MethodPatch, "/api/quota-exceeded/switch-project"),
		routeKey(http.MethodPatch, "/api/api-keys"),
		routeKey(http.MethodDelete, "/api/api-keys"),
		routeKey(http.MethodGet, "/api/codex-api-key"),
		routeKey(http.MethodPut, "/api/codex-api-key"),
		routeKey(http.MethodPatch, "/api/codex-api-key"),
		routeKey(http.MethodDelete, "/api/codex-api-key"),
		routeKey(http.MethodPost, "/api/api-call"),
		routeKey(http.MethodGet, "/api/auth-files/download"),
		routeKey(http.MethodPatch, "/api/auth-files/status"),
		routeKey(http.MethodPatch, "/api/auth-files/fields"),
		routeKey(http.MethodGet, "/api/codex-auth-url"),
		routeKey(http.MethodPost, "/api/oauth-callback"),
		routeKey(http.MethodGet, "/api/get-auth-status"),
		routeKey(http.MethodGet, "/codex/callback"),
	}

	for _, route := range removedRoutes {
		if _, ok := routes[route]; ok {
			t.Fatalf("expected removed route %s to be absent; mounted routes: %v", route, sortedRouteKeys(routes))
		}
	}
}

func TestManagementRetainedRoutesRemainMounted(t *testing.T) {
	server := newTestServer(t, nil)
	routes := mountedRouteSet(server)

	retainedRoutes := []string{
		routeKey(http.MethodGet, "/api/runtime-settings"),
		routeKey(http.MethodPut, "/api/runtime-settings"),
		routeKey(http.MethodGet, "/api/api-keys"),
		routeKey(http.MethodPut, "/api/api-keys"),
		routeKey(http.MethodGet, "/api/auth-files"),
		routeKey(http.MethodPost, "/api/auth-files"),
		routeKey(http.MethodGet, "/api/auth-files/:name/content"),
		routeKey(http.MethodPatch, "/api/auth-files/:name"),
		routeKey(http.MethodDelete, "/api/auth-files/:name"),
		routeKey(http.MethodPost, "/api/auth-files/:name/usage"),
		routeKey(http.MethodPost, "/api/oauth-sessions"),
		routeKey(http.MethodGet, "/api/oauth-sessions/:state"),
		routeKey(http.MethodGet, "/auth/callback"),
	}

	for _, route := range retainedRoutes {
		if _, ok := routes[route]; !ok {
			t.Fatalf("expected retained route %s to stay mounted; mounted routes: %v", route, sortedRouteKeys(routes))
		}
	}
}
