package management

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/gin-gonic/gin"
)

func TestPatchCodexKey_UpdatesRetainedFields(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(&config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:     "existing-key",
			BaseURL:    "https://example.invalid/v1",
			Priority:   1,
			Websockets: false,
			Headers:    map[string]string{"Existing": "value"},
		}},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/codex-api-key", bytes.NewBufferString(`{"index":0,"value":{"api-key":" next-key ","base-url":" https://next.invalid/v1 ","priority":7,"websockets":true,"headers":{" X-Test ":" ok ","Drop":"   "}}}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	h.PatchCodexKey(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if len(h.cfg.CodexKey) != 1 {
		t.Fatalf("expected one codex key after patch, got %d", len(h.cfg.CodexKey))
	}
	got := h.cfg.CodexKey[0]
	if got.APIKey != "next-key" {
		t.Fatalf("expected api-key to be trimmed and updated, got %q", got.APIKey)
	}
	if got.BaseURL != "https://next.invalid/v1" {
		t.Fatalf("expected base-url to be trimmed and updated, got %q", got.BaseURL)
	}
	if got.Priority != 7 {
		t.Fatalf("expected priority to be updated, got %d", got.Priority)
	}
	if !got.Websockets {
		t.Fatal("expected websockets to be updated")
	}
	if len(got.Headers) != 1 || got.Headers["X-Test"] != "ok" {
		t.Fatalf("expected normalized headers to be updated, got %#v", got.Headers)
	}
}

func TestPatchCodexKey_MatchNormalizesStoredAPIKey(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(&config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:   " existing-key ",
			BaseURL:  "https://example.invalid/v1",
			Priority: 1,
		}},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/codex-api-key", bytes.NewBufferString(`{"match":"existing-key","value":{"priority":7}}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	h.PatchCodexKey(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if got := h.cfg.CodexKey[0].APIKey; got != "existing-key" {
		t.Fatalf("expected stored api-key to be normalized, got %q", got)
	}
	if got := h.cfg.CodexKey[0].Priority; got != 7 {
		t.Fatalf("expected priority to update through normalized match, got %d", got)
	}
}

func TestDeleteCodexKey_NormalizesStoredAndRequestedAPIKey(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(&config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:  " existing-key ",
			BaseURL: "https://example.invalid/v1",
		}},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodDelete, "/v0/management/codex-api-key?api-key=%20existing-key%20", nil)
	ctx.Request = req

	h.DeleteCodexKey(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if len(h.cfg.CodexKey) != 0 {
		t.Fatalf("expected codex key to be deleted, got %#v", h.cfg.CodexKey)
	}
}

func TestPatchCodexKey_BlankBaseURLRejected(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(&config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:  "existing-key",
			BaseURL: "https://example.invalid/v1",
		}},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/codex-api-key", bytes.NewBufferString(`{"index":0,"value":{"base-url":"   "}}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	h.PatchCodexKey(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if len(h.cfg.CodexKey) != 1 || h.cfg.CodexKey[0].BaseURL != "https://example.invalid/v1" {
		t.Fatalf("expected codex key to remain unchanged, got %#v", h.cfg.CodexKey)
	}
}

func TestPutCodexKeys_BlankBaseURLRejected(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(&config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:  "existing-key",
			BaseURL: "https://example.invalid/v1",
		}},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPut, "/v0/management/codex-api-key", bytes.NewBufferString(`[{"api-key":"next-key","base-url":"   "}]`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	h.PutCodexKeys(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if len(h.cfg.CodexKey) != 1 || h.cfg.CodexKey[0].APIKey != "existing-key" {
		t.Fatalf("expected existing codex key to remain unchanged, got %#v", h.cfg.CodexKey)
	}
}
