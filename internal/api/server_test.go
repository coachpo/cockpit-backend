package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	proxyconfig "github.com/coachpo/cockpit-backend/internal/config"
	sdkconfig "github.com/coachpo/cockpit-backend/internal/config"
	sdkaccess "github.com/coachpo/cockpit-backend/sdk/access"
	"github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	gin "github.com/gin-gonic/gin"
)

func newTestServer(t *testing.T, mutate func(*proxyconfig.Config), opts ...ServerOption) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port:    0,
		AuthDir: authDir,
	}
	if mutate != nil {
		mutate(cfg)
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 0\n"), 0o600); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}
	return NewServer(cfg, authManager, accessManager, configPath, nil, opts...)
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

func TestManagementRetainedRouteIsAccessibleWithoutAuthorization(t *testing.T) {
	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/v0/management/runtime-settings",
	}, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime-settings route status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestManagementRetainedRouteIgnoresManagementPasswordEnv(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "secret")

	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/v0/management/runtime-settings",
	}, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime-settings route status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
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

func TestManagementRemovedRoutesAreNotMounted(t *testing.T) {
	server := newTestServer(t, nil)
	routes := mountedRouteSet(server)

	removedRoutes := []string{
		routeKey(http.MethodGet, "/v0/management/ws-auth"),
		routeKey(http.MethodPut, "/v0/management/ws-auth"),
		routeKey(http.MethodPatch, "/v0/management/ws-auth"),
		routeKey(http.MethodGet, "/v0/management/request-retry"),
		routeKey(http.MethodPut, "/v0/management/request-retry"),
		routeKey(http.MethodPatch, "/v0/management/request-retry"),
		routeKey(http.MethodGet, "/v0/management/max-retry-interval"),
		routeKey(http.MethodPut, "/v0/management/max-retry-interval"),
		routeKey(http.MethodPatch, "/v0/management/max-retry-interval"),
		routeKey(http.MethodGet, "/v0/management/routing/strategy"),
		routeKey(http.MethodPut, "/v0/management/routing/strategy"),
		routeKey(http.MethodPatch, "/v0/management/routing/strategy"),
		routeKey(http.MethodGet, "/v0/management/quota-exceeded/switch-project"),
		routeKey(http.MethodPut, "/v0/management/quota-exceeded/switch-project"),
		routeKey(http.MethodPatch, "/v0/management/quota-exceeded/switch-project"),
		routeKey(http.MethodPatch, "/v0/management/api-keys"),
		routeKey(http.MethodDelete, "/v0/management/api-keys"),
		routeKey(http.MethodGet, "/v0/management/codex-api-key"),
		routeKey(http.MethodPut, "/v0/management/codex-api-key"),
		routeKey(http.MethodPatch, "/v0/management/codex-api-key"),
		routeKey(http.MethodDelete, "/v0/management/codex-api-key"),
		routeKey(http.MethodPost, "/v0/management/api-call"),
		routeKey(http.MethodGet, "/v0/management/auth-files/download"),
		routeKey(http.MethodPatch, "/v0/management/auth-files/status"),
		routeKey(http.MethodPatch, "/v0/management/auth-files/fields"),
		routeKey(http.MethodGet, "/v0/management/codex-auth-url"),
		routeKey(http.MethodPost, "/v0/management/oauth-callback"),
		routeKey(http.MethodGet, "/v0/management/get-auth-status"),
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
		routeKey(http.MethodGet, "/v0/management/runtime-settings"),
		routeKey(http.MethodPut, "/v0/management/runtime-settings"),
		routeKey(http.MethodGet, "/v0/management/api-keys"),
		routeKey(http.MethodPut, "/v0/management/api-keys"),
		routeKey(http.MethodGet, "/v0/management/auth-files"),
		routeKey(http.MethodPost, "/v0/management/auth-files"),
		routeKey(http.MethodGet, "/v0/management/auth-files/:name/content"),
		routeKey(http.MethodPatch, "/v0/management/auth-files/:name"),
		routeKey(http.MethodDelete, "/v0/management/auth-files/:name"),
		routeKey(http.MethodPost, "/v0/management/auth-files/:name/usage"),
		routeKey(http.MethodPost, "/v0/management/oauth-sessions"),
		routeKey(http.MethodGet, "/v0/management/oauth-sessions/:state"),
		routeKey(http.MethodPost, "/v0/management/oauth-sessions/:state/callback"),
	}

	for _, route := range retainedRoutes {
		if _, ok := routes[route]; !ok {
			t.Fatalf("expected retained route %s to stay mounted; mounted routes: %v", route, sortedRouteKeys(routes))
		}
	}
}
