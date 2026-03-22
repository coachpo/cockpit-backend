package management

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func hashMiddlewareSecret(t *testing.T, secret string) string {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash middleware secret: %v", err)
	}
	return string(hashed)
}

func newMiddlewareTestRouter(t *testing.T, cfg *config.Config) (*gin.Engine, *Handler) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	if cfg == nil {
		cfg = &config.Config{}
	}
	h := NewHandlerWithoutConfigFilePath(cfg, nil)
	router := gin.New()
	router.GET("/v0/management/test", h.Middleware(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router, h
}

func performMiddlewareRequest(router *gin.Engine, authorization string, xManagementKey string, remoteAddr string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/management/test", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if xManagementKey != "" {
		req.Header.Set("X-Management-Key", xManagementKey)
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	router.ServeHTTP(rec, req)
	return rec
}

func TestMiddlewareRequiresBearerAuthorizationHeader(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	router, _ := newMiddlewareTestRouter(t, &config.Config{
		RemoteManagement: config.RemoteManagement{
			AllowRemote: true,
			SecretKey:   hashMiddlewareSecret(t, "secret"),
		},
	})

	tests := []struct {
		name           string
		authorization  string
		xManagementKey string
		wantCode       int
		wantBody       string
	}{
		{name: "accepts bearer token", authorization: "Bearer secret", wantCode: http.StatusNoContent},
		{name: "rejects raw authorization token", authorization: "secret", wantCode: http.StatusUnauthorized, wantBody: "missing management key"},
		{name: "rejects x management key header", xManagementKey: "secret", wantCode: http.StatusUnauthorized, wantBody: "missing management key"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := performMiddlewareRequest(router, tc.authorization, tc.xManagementKey, "198.51.100.25:1234")
			if rec.Code != tc.wantCode {
				t.Fatalf("expected status %d, got %d with body %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("expected body to contain %q, got %s", tc.wantBody, rec.Body.String())
			}
		})
	}
}

func TestMiddlewareAllowsRemoteBearerAuthWhenAllowRemoteDisabled(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	router, _ := newMiddlewareTestRouter(t, &config.Config{
		RemoteManagement: config.RemoteManagement{
			AllowRemote: false,
			SecretKey:   hashMiddlewareSecret(t, "secret"),
		},
	})

	rec := performMiddlewareRequest(router, "Bearer secret", "", "203.0.113.9:1234")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}

func TestHandlerDoesNotExposeLocalPasswordCompatibilityShim(t *testing.T) {
	if _, ok := reflect.TypeFor[*Handler]().MethodByName("SetLocalPassword"); ok {
		t.Fatalf("expected Handler not to expose SetLocalPassword compatibility shim")
	}
}

func TestMiddlewareDoesNotBanRepeatedInvalidRequests(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	router, _ := newMiddlewareTestRouter(t, &config.Config{
		RemoteManagement: config.RemoteManagement{
			AllowRemote: true,
			SecretKey:   hashMiddlewareSecret(t, "secret"),
		},
	})

	for attempt := 1; attempt <= 6; attempt++ {
		rec := performMiddlewareRequest(router, "Bearer wrong-secret", "", "198.51.100.44:4321")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected status %d, got %d with body %s", attempt, http.StatusUnauthorized, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "invalid management key") {
			t.Fatalf("attempt %d: expected invalid management key error, got %s", attempt, rec.Body.String())
		}
	}
}
