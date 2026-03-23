package management

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/coachpo/cockpit-backend/sdk/cliproxy/executor"
	"github.com/gin-gonic/gin"
)

type apiCallTestExecutor struct {
	prepare func(req *http.Request, auth *coreauth.Auth) error
	httpDo  func(req *http.Request, auth *coreauth.Auth) (*http.Response, error)
}

func (e apiCallTestExecutor) Identifier() string { return "codex" }

func (e apiCallTestExecutor) Execute(context.Context, *coreauth.Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e apiCallTestExecutor) ExecuteStream(context.Context, *coreauth.Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e apiCallTestExecutor) Refresh(context.Context, *coreauth.Auth) (*coreauth.Auth, error) {
	return nil, nil
}

func (e apiCallTestExecutor) CountTokens(context.Context, *coreauth.Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e apiCallTestExecutor) PrepareRequest(req *http.Request, auth *coreauth.Auth) error {
	if e.prepare != nil {
		return e.prepare(req, auth)
	}
	return nil
}

func (e apiCallTestExecutor) HttpRequest(_ context.Context, auth *coreauth.Auth, req *http.Request) (*http.Response, error) {
	if e.httpDo != nil {
		return e.httpDo(req, auth)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(`{"status":"ok"}`)),
	}, nil
}

func registerManagedCodexAuth(t *testing.T, manager *coreauth.Manager, auth *coreauth.Auth) string {
	t.Helper()
	if auth.Attributes == nil {
		auth.Attributes = map[string]string{}
	}
	auth.Attributes[managedStoreAttribute] = "true"
	if auth.Provider == "" {
		auth.Provider = "codex"
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}
	stored, ok := manager.GetByID(auth.ID)
	if !ok || stored == nil {
		t.Fatalf("expected auth %q to be registered", auth.ID)
	}
	return stored.EnsureIndex()
}

func TestRefreshAuthFileUsage_UsesBackendOwnedProbeAndReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	registerManagedCodexAuth(t, manager, &coreauth.Auth{
		ID:       "usage.json",
		FileName: "usage.json",
		Metadata: map[string]any{
			"token":    "runtime-token",
			"id_token": testCodexIDToken(t, "usage@example.com", "acct_123", "plus"),
		},
	})

	manager.RegisterExecutor(apiCallTestExecutor{
		prepare: func(req *http.Request, auth *coreauth.Auth) error {
			req.Header.Set("Authorization", "Bearer runtime-token")
			req.Header.Set("X-Prepared-Auth", auth.ID)
			return nil
		},
		httpDo: func(req *http.Request, auth *coreauth.Auth) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", req.Method)
			}
			if req.URL.String() != "https://chatgpt.com/backend-api/wham/usage" {
				t.Fatalf("unexpected target url %q", req.URL.String())
			}
			if got := req.Header.Get("Authorization"); got != "Bearer runtime-token" {
				t.Fatalf("expected prepared authorization header, got %q", got)
			}
			if got := req.Header.Get("Chatgpt-Account-Id"); got != "acct_123" {
				t.Fatalf("expected account header acct_123, got %q", got)
			}
			if got := req.Header.Get("X-Prepared-Auth"); got != auth.ID {
				t.Fatalf("expected prepared auth id %q, got %q", auth.ID, got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewBufferString(`{"remaining":42,"plan":"plus"}`)),
			}, nil
		},
	})

	h := NewHandlerWithoutConfigFilePath(&config.Config{}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/usage.json/usage", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "usage.json"}}

	h.RefreshAuthFileUsage(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected usage status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode usage response: %v", err)
	}
	if got := payload["remaining"]; got != float64(42) {
		t.Fatalf("expected remaining=42, got %#v", payload)
	}
	stored, ok := manager.GetByID("usage.json")
	if !ok || stored == nil {
		t.Fatalf("expected auth to remain registered")
	}
	usage, _ := stored.Metadata["usage"].(map[string]any)
	if got := usage["remaining"]; got != float64(42) {
		t.Fatalf("expected refreshed usage to be persisted, got %#v", stored.Metadata)
	}
}

func TestRefreshAuthFileUsage_ReturnsNotFoundWhenUsageUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	registerManagedCodexAuth(t, manager, &coreauth.Auth{ID: "usage.json", FileName: "usage.json", Provider: "other"})
	h := NewHandlerWithoutConfigFilePath(&config.Config{}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/usage.json/usage", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "usage.json"}}

	h.RefreshAuthFileUsage(ctx)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected usage status %d, got %d with body %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestRefreshAuthFileUsage_ReturnsNotFoundForUnknownAuthFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandlerWithoutConfigFilePath(&config.Config{}, coreauth.NewManager(nil, nil, nil))
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/missing.json/usage", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "missing.json"}}

	h.RefreshAuthFileUsage(ctx)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected usage status %d, got %d with body %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestRefreshAuthFileUsage_PropagatesUpstreamErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	registerManagedCodexAuth(t, manager, &coreauth.Auth{
		ID:       "usage.json",
		FileName: "usage.json",
		Metadata: map[string]any{"id_token": testCodexIDToken(t, "usage@example.com", "acct_123", "plus")},
	})
	manager.RegisterExecutor(apiCallTestExecutor{
		httpDo: func(req *http.Request, auth *coreauth.Auth) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewBufferString(`{"error":"quota exceeded"}`)),
			}, nil
		},
	})

	h := NewHandlerWithoutConfigFilePath(&config.Config{}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/usage.json/usage", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "usage.json"}}

	h.RefreshAuthFileUsage(ctx)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected usage status %d, got %d with body %s", http.StatusTooManyRequests, rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != `{"error":"quota exceeded"}` {
		t.Fatalf("expected upstream body to pass through, got %s", got)
	}
}
