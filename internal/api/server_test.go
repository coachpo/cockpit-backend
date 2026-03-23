package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	managementhandlers "github.com/coachpo/cockpit-backend/internal/api/handlers/management"
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

func TestManagementRetainedRouteIsAccessibleWithoutAuthorization(t *testing.T) {
	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/v0/management/ws-auth",
	}, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected ws-auth route status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestManagementRetainedRouteIgnoresManagementPasswordEnv(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "secret")

	server := newTestServer(t, nil)

	rec := performManagementRequest(server, managementRouteCase{
		method: http.MethodGet,
		path:   "/v0/management/ws-auth",
	}, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected ws-auth route status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
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
	removedRoutes := []managementRouteCase{
		{name: "config get", method: http.MethodGet, path: "/v0/management/config"},
		{name: "config yaml get", method: http.MethodGet, path: "/v0/management/config.yaml"},
		{name: "config yaml put", method: http.MethodPut, path: "/v0/management/config.yaml", body: "debug: true\n", contentType: "application/yaml"},
		{name: "latest version", method: http.MethodGet, path: "/v0/management/latest-version"},
		{name: "debug get", method: http.MethodGet, path: "/v0/management/debug"},
		{name: "debug put", method: http.MethodPut, path: "/v0/management/debug", body: `{"value":true}`, contentType: "application/json"},
		{name: "debug patch", method: http.MethodPatch, path: "/v0/management/debug", body: `{"value":true}`, contentType: "application/json"},
		{name: "request log get", method: http.MethodGet, path: "/v0/management/request-log"},
		{name: "request log put", method: http.MethodPut, path: "/v0/management/request-log", body: `{"value":true}`, contentType: "application/json"},
		{name: "request log patch", method: http.MethodPatch, path: "/v0/management/request-log", body: `{"value":true}`, contentType: "application/json"},
		{name: "proxy url get", method: http.MethodGet, path: "/v0/management/proxy-url"},
		{name: "proxy url put", method: http.MethodPut, path: "/v0/management/proxy-url", body: `{"value":"http://127.0.0.1:9000"}`, contentType: "application/json"},
		{name: "proxy url patch", method: http.MethodPatch, path: "/v0/management/proxy-url", body: `{"value":"http://127.0.0.1:9000"}`, contentType: "application/json"},
		{name: "proxy url delete", method: http.MethodDelete, path: "/v0/management/proxy-url"},
		{name: "quota preview get", method: http.MethodGet, path: "/v0/management/quota-exceeded/switch-preview-model"},
		{name: "quota preview put", method: http.MethodPut, path: "/v0/management/quota-exceeded/switch-preview-model", body: `{"value":true}`, contentType: "application/json"},
		{name: "quota preview patch", method: http.MethodPatch, path: "/v0/management/quota-exceeded/switch-preview-model", body: `{"value":true}`, contentType: "application/json"},
		{name: "force model prefix get", method: http.MethodGet, path: "/v0/management/force-model-prefix"},
		{name: "force model prefix put", method: http.MethodPut, path: "/v0/management/force-model-prefix", body: `{"value":true}`, contentType: "application/json"},
		{name: "force model prefix patch", method: http.MethodPatch, path: "/v0/management/force-model-prefix", body: `{"value":true}`, contentType: "application/json"},
		{name: "oauth excluded models get", method: http.MethodGet, path: "/v0/management/oauth-excluded-models"},
		{name: "oauth excluded models put", method: http.MethodPut, path: "/v0/management/oauth-excluded-models", body: `{}`, contentType: "application/json"},
		{name: "oauth excluded models patch", method: http.MethodPatch, path: "/v0/management/oauth-excluded-models", body: `{"provider":"codex","models":["gpt-5"]}`, contentType: "application/json"},
		{name: "oauth excluded models delete", method: http.MethodDelete, path: "/v0/management/oauth-excluded-models?provider=codex"},
		{name: "oauth model alias get", method: http.MethodGet, path: "/v0/management/oauth-model-alias"},
		{name: "oauth model alias put", method: http.MethodPut, path: "/v0/management/oauth-model-alias", body: `{}`, contentType: "application/json"},
		{name: "oauth model alias patch", method: http.MethodPatch, path: "/v0/management/oauth-model-alias", body: `{"provider":"codex","aliases":[]}`, contentType: "application/json"},
		{name: "oauth model alias delete", method: http.MethodDelete, path: "/v0/management/oauth-model-alias?provider=codex"},
		{name: "model definitions", method: http.MethodGet, path: "/v0/management/model-definitions/codex"},
	}

	for _, route := range removedRoutes {
		t.Run(route.name, func(t *testing.T) {
			server := newTestServer(t, nil)
			rec := performManagementRequest(server, route, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected removed route %s %s to return %d, got %d with body %s", route.method, route.path, http.StatusNotFound, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestManagementRetainedRoutesRemainMounted(t *testing.T) {
	retainedRoutes := []managementRouteCase{
		{name: "ws auth get", method: http.MethodGet, path: "/v0/management/ws-auth"},
		{name: "ws auth put", method: http.MethodPut, path: "/v0/management/ws-auth", body: `{"value":true}`, contentType: "application/json"},
		{name: "ws auth patch", method: http.MethodPatch, path: "/v0/management/ws-auth", body: `{"value":true}`, contentType: "application/json"},
		{name: "request retry get", method: http.MethodGet, path: "/v0/management/request-retry"},
		{name: "request retry put", method: http.MethodPut, path: "/v0/management/request-retry", body: `{"value":1}`, contentType: "application/json"},
		{name: "request retry patch", method: http.MethodPatch, path: "/v0/management/request-retry", body: `{"value":1}`, contentType: "application/json"},
		{name: "max retry interval get", method: http.MethodGet, path: "/v0/management/max-retry-interval"},
		{name: "max retry interval put", method: http.MethodPut, path: "/v0/management/max-retry-interval", body: `{"value":30}`, contentType: "application/json"},
		{name: "max retry interval patch", method: http.MethodPatch, path: "/v0/management/max-retry-interval", body: `{"value":30}`, contentType: "application/json"},
		{name: "routing strategy get", method: http.MethodGet, path: "/v0/management/routing/strategy"},
		{name: "routing strategy put", method: http.MethodPut, path: "/v0/management/routing/strategy", body: `{"value":"round-robin"}`, contentType: "application/json"},
		{name: "routing strategy patch", method: http.MethodPatch, path: "/v0/management/routing/strategy", body: `{"value":"round-robin"}`, contentType: "application/json"},
		{name: "quota project get", method: http.MethodGet, path: "/v0/management/quota-exceeded/switch-project"},
		{name: "quota project put", method: http.MethodPut, path: "/v0/management/quota-exceeded/switch-project", body: `{"value":true}`, contentType: "application/json"},
		{name: "quota project patch", method: http.MethodPatch, path: "/v0/management/quota-exceeded/switch-project", body: `{"value":true}`, contentType: "application/json"},
		{name: "api keys get", method: http.MethodGet, path: "/v0/management/api-keys"},
		{name: "api keys put", method: http.MethodPut, path: "/v0/management/api-keys", body: `["test-key"]`, contentType: "application/json"},
		{name: "api keys patch", method: http.MethodPatch, path: "/v0/management/api-keys", body: `{"old":"test-key","new":"updated-key"}`, contentType: "application/json"},
		{name: "api keys delete", method: http.MethodDelete, path: "/v0/management/api-keys?value=test-key"},
		{name: "codex api key get", method: http.MethodGet, path: "/v0/management/codex-api-key"},
		{name: "codex api key put", method: http.MethodPut, path: "/v0/management/codex-api-key", body: `[]`, contentType: "application/json"},
		{name: "codex api key patch", method: http.MethodPatch, path: "/v0/management/codex-api-key", body: `{"index":0,"value":{"base-url":""}}`, contentType: "application/json"},
		{name: "codex api key delete", method: http.MethodDelete, path: "/v0/management/codex-api-key?index=0"},
		{name: "auth files get", method: http.MethodGet, path: "/v0/management/auth-files"},
		{name: "auth files upload", method: http.MethodPost, path: "/v0/management/auth-files?name=upload.json", body: `{"type":"codex"}`, contentType: "application/json"},
		{name: "auth files delete", method: http.MethodDelete, path: "/v0/management/auth-files?name=upload.json"},
		{name: "auth files download", method: http.MethodGet, path: "/v0/management/auth-files/download?name=upload.json"},
		{name: "auth files status", method: http.MethodPatch, path: "/v0/management/auth-files/status", body: `{"name":"upload.json","disabled":true}`, contentType: "application/json"},
		{name: "auth files fields", method: http.MethodPatch, path: "/v0/management/auth-files/fields", body: `{"name":"upload.json","priority":7}`, contentType: "application/json"},
		{name: "api call", method: http.MethodPost, path: "/v0/management/api-call", body: `{"method":"GET","url":"https://example.com"}`, contentType: "application/json"},
		{name: "oauth start", method: http.MethodGet, path: "/v0/management/codex-auth-url"},
		{name: "oauth callback", method: http.MethodPost, path: "/v0/management/oauth-callback", body: `{"provider":"codex","state":"state-123","code":"auth-code"}`, contentType: "application/json"},
		{name: "oauth status", method: http.MethodGet, path: "/v0/management/get-auth-status"},
	}

	for _, route := range retainedRoutes {
		t.Run(route.name, func(t *testing.T) {
			server := newTestServer(t, func(cfg *proxyconfig.Config) {
				cfg.CodexKey = []proxyconfig.CodexKey{{
					APIKey:  "existing-key",
					BaseURL: "https://example.com",
				}}
			})
			if route.name == "oauth callback" {
				managementhandlers.RegisterOAuthSession("state-123", "codex")
				t.Cleanup(func() {
					managementhandlers.CompleteOAuthSession("state-123")
				})
			}
			rec := performManagementRequest(server, route, "")
			if rec.Code == http.StatusNotFound {
				t.Fatalf("expected retained route %s %s to stay mounted, got %d with body %s", route.method, route.path, rec.Code, rec.Body.String())
			}
		})
	}
}
