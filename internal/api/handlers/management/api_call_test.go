package management

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("boom") }
func (errReadCloser) Close() error             { return nil }

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

func TestAPICall_UsesAuthProbeRequestAndReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	index := registerManagedCodexAuth(t, manager, &coreauth.Auth{
		ID:       "usage.json",
		FileName: "usage.json",
		Metadata: map[string]any{"token": "runtime-token"},
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
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/api-call", bytes.NewBufferString(`{"authIndex":"`+index+`","method":"GET","url":"https://chatgpt.com/backend-api/wham/usage","header":{"Authorization":"Bearer $TOKEN$","Chatgpt-Account-Id":"acct_123"}}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.APICall(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected api-call status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode api-call response: %v", err)
	}
	if got := payload["remaining"]; got != float64(42) {
		t.Fatalf("expected remaining=42, got %#v", payload)
	}
}

func TestAPICall_ReturnsServiceUnavailableWithoutAuthManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandlerWithoutConfigFilePath(&config.Config{}, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/api-call", bytes.NewBufferString(`{"authIndex":"abc","method":"GET","url":"https://example.com"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.APICall(ctx)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected api-call status %d, got %d with body %s", http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
}

func TestAPICall_ReturnsBadGatewayForUpstreamFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("request error", func(t *testing.T) {
		manager := coreauth.NewManager(nil, nil, nil)
		index := registerManagedCodexAuth(t, manager, &coreauth.Auth{ID: "usage.json", FileName: "usage.json"})
		manager.RegisterExecutor(apiCallTestExecutor{
			httpDo: func(req *http.Request, auth *coreauth.Auth) (*http.Response, error) {
				return nil, errors.New("upstream request failed")
			},
		})

		h := NewHandlerWithoutConfigFilePath(&config.Config{}, manager)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/api-call", bytes.NewBufferString(`{"authIndex":"`+index+`","method":"GET","url":"https://example.com"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")

		h.APICall(ctx)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected api-call status %d, got %d with body %s", http.StatusBadGateway, rec.Code, rec.Body.String())
		}
	})

	t.Run("body read error", func(t *testing.T) {
		manager := coreauth.NewManager(nil, nil, nil)
		index := registerManagedCodexAuth(t, manager, &coreauth.Auth{ID: "usage.json", FileName: "usage.json"})
		manager.RegisterExecutor(apiCallTestExecutor{
			httpDo: func(req *http.Request, auth *coreauth.Auth) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       errReadCloser{},
				}, nil
			},
		})

		h := NewHandlerWithoutConfigFilePath(&config.Config{}, manager)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/api-call", bytes.NewBufferString(`{"authIndex":"`+index+`","method":"GET","url":"https://example.com"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")

		h.APICall(ctx)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected api-call status %d, got %d with body %s", http.StatusBadGateway, rec.Code, rec.Body.String())
		}
	})
}
