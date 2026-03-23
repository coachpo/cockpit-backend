package nacos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

func TestStaticConfigSourceSaveConfigWritesFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	source := NewStaticConfigSource(configPath)

	cfg := &config.Config{
		Port:         8317,
		RequestRetry: 4,
	}

	if err := source.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if strings.Contains(string(raw), "remote-management") {
		t.Fatalf("expected saved config to omit removed remote-management block, got %s", string(raw))
	}

	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to reload saved config: %v", err)
	}
	if loaded.Port != 8317 {
		t.Fatalf("expected port 8317, got %d", loaded.Port)
	}
	if loaded.RequestRetry != 4 {
		t.Fatalf("expected request retry 4, got %d", loaded.RequestRetry)
	}
}

func TestStaticConfigSourceSaveConfigRejectsCodexKeyWithoutBaseURL(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	source := NewStaticConfigSource(configPath)

	err := source.SaveConfig(&config.Config{CodexKey: []config.CodexKey{{APIKey: "test", BaseURL: "   "}}})
	if err == nil {
		t.Fatal("expected SaveConfig to reject codex key without base-url")
	}
	if !strings.Contains(err.Error(), "codex-api-key[0].base-url is required") {
		t.Fatalf("expected base-url validation error, got %v", err)
	}
}

func TestStaticAuthStoreSaveAndDeleteWritesLocalFiles(t *testing.T) {
	authDir := t.TempDir()
	store := NewStaticAuthStore(authDir)

	auth := &coreauth.Auth{
		ID:       "demo.json",
		FileName: "demo.json",
		Provider: "codex",
		Prefix:   "team-a",
		ProxyURL: "http://127.0.0.1:9000",
		Disabled: true,
		Metadata: map[string]any{
			"type":     "codex",
			"email":    "demo@example.com",
			"priority": 7,
			"note":     "hello",
		},
	}

	savedPath, err := store.Save(context.Background(), auth)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	expectedPath := filepath.Join(authDir, "demo.json")
	if savedPath != expectedPath {
		t.Fatalf("expected saved path %q, got %q", expectedPath, savedPath)
	}

	raw, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read saved auth file: %v", err)
	}

	var payload map[string]any
	if err = json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("failed to decode saved auth payload: %v", err)
	}
	if payload["type"] != "codex" {
		t.Fatalf("expected type codex, got %#v", payload["type"])
	}
	if payload["prefix"] != "team-a" {
		t.Fatalf("expected prefix team-a, got %#v", payload["prefix"])
	}
	if payload["proxy_url"] != "http://127.0.0.1:9000" {
		t.Fatalf("expected proxy_url to persist, got %#v", payload["proxy_url"])
	}
	if payload["disabled"] != true {
		t.Fatalf("expected disabled=true, got %#v", payload["disabled"])
	}

	if err = store.Delete(context.Background(), "demo.json"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err = os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("expected auth file to be removed, stat err=%v", err)
	}
}
