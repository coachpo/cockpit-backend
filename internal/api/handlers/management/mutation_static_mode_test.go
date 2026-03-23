package management

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

func TestUploadAuthFile_UsesInjectedStoreWithoutWritingDisk(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	manager := coreauth.NewManager(nil, nil, nil)
	store := &recordingAuthStore{}
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.authStore = store

	uploadRec := httptest.NewRecorder()
	uploadCtx, _ := gin.CreateTestContext(uploadRec)
	uploadReq := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files?name=upload.json", bytes.NewBufferString(`{"type":"codex","email":"upload@example.com"}`))
	uploadReq.Header.Set("Content-Type", "application/json")
	uploadCtx.Request = uploadReq
	h.UploadAuthFile(uploadCtx)

	if uploadRec.Code != http.StatusOK {
		t.Fatalf("expected upload status %d, got %d with body %s", http.StatusOK, uploadRec.Code, uploadRec.Body.String())
	}
	if saved := store.lastSaved(); saved == nil {
		t.Fatalf("expected injected store to receive saved auth record")
	} else {
		if saved.ID != "upload.json" {
			t.Fatalf("expected saved auth id upload.json, got %q", saved.ID)
		}
		if saved.FileName != "upload.json" {
			t.Fatalf("expected saved auth filename upload.json, got %q", saved.FileName)
		}
	}
	entries, errReadDir := os.ReadDir(authDir)
	if errReadDir != nil {
		t.Fatalf("failed to read auth dir: %v", errReadDir)
	}
	if len(entries) != 0 {
		t.Fatalf("expected upload not to write auth files to disk, found %d entries", len(entries))
	}
	if _, ok := manager.GetByID("upload.json"); !ok {
		t.Fatalf("expected auth manager to be updated after upload")
	}
}

func TestPatchAuthFileStatus_StaticModePersistsAndUpdatesManager(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	filePath := filepath.Join(authDir, "status.json")
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"codex","email":"status@example.com"}`), 0o600); errWrite != nil {
		t.Fatalf("failed to seed auth file: %v", errWrite)
	}
	manager := coreauth.NewManager(nil, nil, nil)
	if _, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "status.json",
		FileName: "status.json",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			managedStoreAttribute: "true",
		},
		Metadata: map[string]any{"type": "codex"},
	}); errRegister != nil {
		t.Fatalf("failed to register auth: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.authStore = nacos.NewStaticAuthStore(authDir)

	patchRec := httptest.NewRecorder()
	patchCtx, _ := gin.CreateTestContext(patchRec)
	patchReq := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/status", bytes.NewBufferString(`{"name":"status.json","disabled":true}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchCtx.Request = patchReq
	h.PatchAuthFileStatus(patchCtx)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch status code %d, got %d with body %s", http.StatusOK, patchRec.Code, patchRec.Body.String())
	}
	updated, ok := manager.GetByID("status.json")
	if !ok {
		t.Fatalf("expected auth to remain available in manager")
	}
	if !updated.Disabled || updated.Status != coreauth.StatusDisabled {
		t.Fatalf("expected auth to be disabled after patch, got %+v", updated)
	}
	raw, errRead := os.ReadFile(filePath)
	if errRead != nil {
		t.Fatalf("failed to read updated auth file: %v", errRead)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("failed to decode updated auth file: %v", err)
	}
	if saved["disabled"] != true {
		t.Fatalf("expected updated auth file to persist disabled=true, got %+v", saved)
	}
}

func TestPatchAuthFileFields_StaticModePersistsAndUpdatesManager(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	filePath := filepath.Join(authDir, "fields.json")
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"codex","email":"fields@example.com"}`), 0o600); errWrite != nil {
		t.Fatalf("failed to seed auth file: %v", errWrite)
	}
	manager := coreauth.NewManager(nil, nil, nil)
	if _, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "fields.json",
		FileName: "fields.json",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			managedStoreAttribute: "true",
		},
		Metadata: map[string]any{"type": "codex", "email": "fields@example.com"},
	}); errRegister != nil {
		t.Fatalf("failed to register auth: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.authStore = nacos.NewStaticAuthStore(authDir)

	patchRec := httptest.NewRecorder()
	patchCtx, _ := gin.CreateTestContext(patchRec)
	patchReq := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/fields", bytes.NewBufferString(`{"name":"fields.json","priority":7}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchCtx.Request = patchReq
	h.PatchAuthFileFields(patchCtx)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch fields code %d, got %d with body %s", http.StatusOK, patchRec.Code, patchRec.Body.String())
	}
	updated, ok := manager.GetByID("fields.json")
	if !ok {
		t.Fatalf("expected auth to remain available in manager")
	}
	if updated.Prefix != "" {
		t.Fatalf("expected prefix to remain empty, got %q", updated.Prefix)
	}
	if updated.Metadata == nil || updated.Metadata["priority"] == nil {
		t.Fatalf("expected metadata priority to be updated, got %+v", updated.Metadata)
	}
	if _, ok := updated.Metadata["note"]; ok {
		t.Fatalf("did not expect note metadata to be updated, got %+v", updated.Metadata)
	}
	raw, errRead := os.ReadFile(filePath)
	if errRead != nil {
		t.Fatalf("failed to read updated auth file: %v", errRead)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("failed to decode updated auth file: %v", err)
	}
	if _, ok := saved["prefix"]; ok {
		t.Fatalf("did not expect prefix to persist, got %+v", saved)
	}
	if _, ok := saved["note"]; ok {
		t.Fatalf("did not expect note to persist, got %+v", saved)
	}
	if saved["priority"] != float64(7) {
		t.Fatalf("expected updated auth file to persist edited fields, got %+v", saved)
	}
}

func TestRequestCodexToken_StaticModeReturnsOAuthStartPayload(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, nil)
	h.authStore = nacos.NewStaticAuthStore(authDir)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/codex-auth-url", nil)
	ctx.Request = req
	h.RequestCodexToken(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected codex auth request status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode oauth start response: %v", err)
	}
	state, _ := body["state"].(string)
	if state == "" {
		t.Fatalf("expected oauth start response to include state, got %s", rec.Body.String())
	}
	CompleteOAuthSession(state)
	if body["status"] != "ok" {
		t.Fatalf("expected oauth start status ok, got %s", rec.Body.String())
	}
}

func TestPostOAuthCallback_DoesNotWriteCallbackFile(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	state := "state-123"
	RegisterOAuthSession(state, "codex")
	defer CompleteOAuthSession(state)

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v0/management/oauth-callback", bytes.NewBufferString(`{"provider":"codex","state":"state-123","code":"auth-code"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	h.PostOAuthCallback(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected oauth callback status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	entries, errReadDir := os.ReadDir(authDir)
	if errReadDir != nil {
		t.Fatalf("failed to read auth dir: %v", errReadDir)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".oauth") {
			t.Fatalf("expected oauth callback to avoid writing callback files, found %s", entry.Name())
		}
	}
	if !IsOAuthSessionPending(state, "codex") {
		t.Fatalf("expected oauth session to remain pending until callback is consumed")
	}
}

func TestListAuthFiles_ExcludesConfigBackedManagerEntries(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	manager := coreauth.NewManager(nil, nil, nil)
	if _, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:         "config-auth",
		Provider:   "codex",
		Label:      "config@example.com",
		Status:     coreauth.StatusActive,
		Attributes: map[string]string{"source": "config:codex"},
		Metadata:   map[string]any{"type": "codex", "email": "config@example.com"},
	}); errRegister != nil {
		t.Fatalf("failed to register config-backed auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:         "managed-auth",
		FileName:   "managed.json",
		Provider:   "codex",
		Label:      "managed@example.com",
		Status:     coreauth.StatusActive,
		Attributes: map[string]string{managedStoreAttribute: "true"},
		Metadata:   map[string]any{"type": "codex", "email": "managed@example.com"},
	}); errRegister != nil {
		t.Fatalf("failed to register managed auth: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
	ctx.Request = req
	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "config-auth") || strings.Contains(body, "config@example.com") {
		t.Fatalf("expected config-backed auth to be excluded, got body %s", body)
	}
	if !strings.Contains(body, "managed-auth") {
		t.Fatalf("expected managed auth to remain listed, got body %s", body)
	}
}

func TestListAuthFiles_ExposesUsageSubscriptionAndActiveFallback(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	manager := coreauth.NewManager(nil, nil, nil)
	if _, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "oauth-auth",
		FileName: "oauth-auth.json",
		Provider: "codex",
		Label:    "oauth@example.com",
		Attributes: map[string]string{
			managedStoreAttribute: "true",
		},
		Metadata: map[string]any{
			"type":     "codex",
			"email":    "oauth@example.com",
			"id_token": testCodexIDToken(t, "oauth@example.com", "acct_456", "team"),
			"usage": map[string]any{
				"remaining": 42,
			},
		},
	}); errRegister != nil {
		t.Fatalf("failed to register managed auth: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Files []struct {
			ID         string         `json:"id"`
			Status     string         `json:"status"`
			Usage      map[string]any `json:"usage"`
			UsageProbe map[string]any `json:"usage_probe"`
			IDToken    map[string]any `json:"id_token"`
			Disabled   bool           `json:"disabled"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode auth files response: %v", err)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 auth file, got %d (%s)", len(payload.Files), rec.Body.String())
	}

	file := payload.Files[0]
	if file.ID != "oauth-auth" {
		t.Fatalf("expected auth id oauth-auth, got %q", file.ID)
	}
	if file.Status != "active" {
		t.Fatalf("expected blank runtime status to normalize to active, got %q", file.Status)
	}
	if file.Disabled {
		t.Fatalf("expected auth file to remain enabled, got disabled=true")
	}
	if got := file.Usage["remaining"]; got != float64(42) {
		t.Fatalf("expected usage payload to be exposed, got %#v", file.Usage)
	}
	if got := file.UsageProbe["authIndex"]; got == nil || got == "" {
		t.Fatalf("expected usage_probe authIndex to be exposed, got %#v", file.UsageProbe)
	}
	if got := file.UsageProbe["url"]; got != "https://chatgpt.com/backend-api/wham/usage" {
		t.Fatalf("expected usage probe url to be set, got %#v", got)
	}
	if got := file.IDToken["plan_type"]; got != "team" {
		t.Fatalf("expected plan_type team, got %#v", got)
	}
	if got := file.IDToken["subscription"]; got != "active" {
		t.Fatalf("expected derived subscription active, got %#v", got)
	}
}
