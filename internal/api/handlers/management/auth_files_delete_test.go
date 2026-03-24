package management

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

func TestDeleteAuthFile_UsesInjectedStoreIDWithoutFilesystemMutation(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "codex-user@example.com-plus.json"
	filePath := filepath.Join(authDir, fileName)
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"codex","email":"real@example.com"}`), 0o600); errWrite != nil {
		t.Fatalf("failed to write auth file: %v", errWrite)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	record := &coreauth.Auth{
		ID:       fileName,
		FileName: fileName,
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			managedStoreAttribute: "true",
		},
		Metadata: map[string]any{
			"type":  "codex",
			"email": "real@example.com",
		},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	store := &recordingAuthStore{}
	h := NewHandlerWithoutPersistence(&config.Config{}, manager)
	h.authStore = store

	deleteRec := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRec)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/auth-files/"+fileName, nil)
	deleteCtx.Request = deleteReq
	deleteCtx.Params = gin.Params{{Key: "name", Value: fileName}}
	h.DeleteAuthFile(deleteCtx)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d with body %s", http.StatusOK, deleteRec.Code, deleteRec.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != fileName {
		t.Fatalf("expected store delete by auth id %q, got %#v", fileName, store.deleted)
	}
	if _, errStat := os.Stat(filePath); errStat != nil {
		t.Fatalf("expected auth file to remain untouched on disk, stat err: %v", errStat)
	}

	listRec := httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(listRec)
	listReq := httptest.NewRequest(http.MethodGet, "/api/auth-files", nil)
	listCtx.Request = listReq
	h.ListAuthFiles(listCtx)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, listRec.Code, listRec.Body.String())
	}
	if strings.Contains(listRec.Body.String(), fileName) {
		t.Fatalf("expected deleted auth to be hidden from list, got body %s", listRec.Body.String())
	}
}

func TestDownloadAuthFile_UsesStoreReadByNameWithoutDisk(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	store := &recordingAuthStore{
		items: map[string]*coreauth.Auth{
			"download.json": {
				ID:       "download.json",
				FileName: "download.json",
				Provider: "codex",
				Label:    "download@example.com",
				Metadata: map[string]any{"type": "codex", "email": "download@example.com"},
			},
		},
	}
	h := NewHandlerWithoutPersistence(&config.Config{}, nil)
	h.authStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/auth-files/download.json/content", nil)
	ctx.Request = req
	ctx.Params = gin.Params{{Key: "name", Value: "download.json"}}
	h.GetAuthFileContent(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected download status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "download@example.com") {
		t.Fatalf("expected store-backed download body, got %s", body)
	}
}

func TestListAuthFiles_ManagerRequired(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	store := &recordingAuthStore{
		items: map[string]*coreauth.Auth{
			"store-list.json": {
				ID:       "store-list.json",
				FileName: "store-list.json",
				Provider: "codex",
				Label:    "store-list@example.com",
				Metadata: map[string]any{"type": "codex", "email": "store-list@example.com", "priority": 7, "note": "hello", "prefix": "team-a"},
			},
		},
	}
	h := NewHandlerWithoutPersistence(&config.Config{}, nil)
	h.authStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/auth-files", nil)
	ctx.Request = req
	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "core auth manager unavailable") {
		t.Fatalf("expected core auth manager unavailable error, got %s", rec.Body.String())
	}
}
