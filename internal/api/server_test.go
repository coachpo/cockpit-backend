package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	proxyconfig "github.com/coachpo/cockpit-backend/internal/config"
	sdkconfig "github.com/coachpo/cockpit-backend/internal/config"
	sdkaccess "github.com/coachpo/cockpit-backend/sdk/access"
	"github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	gin "github.com/gin-gonic/gin"
)

func newTestServer(t *testing.T) *Server {
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
		Debug:   true,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	configPath := filepath.Join(tmpDir, "config.yaml")
	return NewServer(cfg, authManager, accessManager, configPath, nil)
}

func TestManagementRequestLogRouteRegisteredWhenSecretPresent(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "secret")

	server := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/management/request-log", nil)
	req.Header.Set("X-Management-Key", "secret")

	server.engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected request-log route status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}
