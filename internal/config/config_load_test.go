package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigRejectsUnknownRemovedKeys(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "removed root key",
			yaml:    "port: 8080\nrequest-log: true\n",
			wantErr: "request-log",
		},
		{
			name:    "removed nested key",
			yaml:    "port: 8080\nquota-exceeded:\n  switch-project: true\n  switch-preview-model: true\n",
			wantErr: "switch-preview-model",
		},
		{
			name:    "removed remote management block",
			yaml:    "port: 8080\nremote-management:\n  allow-remote: false\n",
			wantErr: "remote-management",
		},
		{
			name:    "removed codex subfield",
			yaml:    "port: 8080\ncodex-api-key:\n  - api-key: test\n    base-url: https://example.invalid/v1\n    proxy-url: https://proxy.invalid\n",
			wantErr: "proxy-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := LoadConfig(configPath)
			if err == nil {
				t.Fatal("expected LoadConfig to reject unknown key")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadConfigRejectsCodexKeyWithoutBaseURL(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "port: 8080\ncodex-api-key:\n  - api-key: test\n    base-url: \"   \"\n"
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected LoadConfig to reject codex key without base-url")
	}
	if !strings.Contains(err.Error(), "codex-api-key") {
		t.Fatalf("expected error mentioning codex-api-key, got %v", err)
	}
}
