package management

import (
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
			"path": filePath,
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
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.authStore = store

	deleteRec := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRec)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v0/management/auth-files/"+fileName, nil)
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
	listReq := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
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

	authDir := t.TempDir()
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
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, nil)
	h.authStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files/download.json/content", nil)
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

func TestListAuthFiles_UsesStoreMetadataWhenManagerUnavailable(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
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
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, nil)
	h.authStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
	ctx.Request = req
	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			Name           string `json:"name"`
			Email          string `json:"email"`
			Priority       int    `json:"priority"`
			UsageAvailable bool   `json:"usage_available"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode auth files response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one store-backed auth file, got %d (%s)", len(payload.Items), rec.Body.String())
	}
	item := payload.Items[0]
	if item.Name != "store-list.json" || item.Email != "store-list@example.com" || item.Priority != 7 {
		t.Fatalf("expected store metadata in list response, got %#v", item)
	}
	body := rec.Body.String()
	if strings.Contains(body, "hello") || strings.Contains(body, "team-a") {
		t.Fatalf("did not expect prefix/note in list response, got %s", body)
	}
}

func TestDeleteAuthFile_StaticModeRejectsBeforeFilesystemMutation(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "readonly-user.json"
	filePath := filepath.Join(authDir, fileName)
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"codex"}`), 0o600); errWrite != nil {
		t.Fatalf("failed to write auth file: %v", errWrite)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	if _, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:       fileName,
		FileName: fileName,
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"path": filePath,
		},
		Metadata: map[string]any{"type": "codex"},
	}); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.authStore = nacos.NewStaticAuthStore(authDir)

	deleteRec := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRec)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v0/management/auth-files/"+fileName, nil)
	deleteCtx.Request = deleteReq
	deleteCtx.Params = gin.Params{{Key: "name", Value: fileName}}
	h.DeleteAuthFile(deleteCtx)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d with body %s", http.StatusOK, deleteRec.Code, deleteRec.Body.String())
	}
	if _, errStat := os.Stat(filePath); !os.IsNotExist(errStat) {
		t.Fatalf("expected auth file to be deleted from disk, stat err: %v", errStat)
	}
	updated, ok := manager.GetByID(fileName)
	if !ok {
		t.Fatalf("expected auth to remain registered after delete for runtime disablement")
	}
	if !updated.Disabled || updated.Status != coreauth.StatusDisabled {
		t.Fatalf("expected auth to be disabled after delete, got %+v", updated)
	}
}
