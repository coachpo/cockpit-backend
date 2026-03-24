package config

import (
	"strings"
	"testing"
)

func TestParseConfigYAMLRejectsUnknownRemovedKeys(t *testing.T) {
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
			_, err := ParseConfigYAML([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected ParseConfigYAML to reject unknown key")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestParseConfigYAMLRejectsCodexKeyWithoutBaseURL(t *testing.T) {
	yaml := "port: 8080\ncodex-api-key:\n  - api-key: test\n    base-url: \"   \"\n"
	_, err := ParseConfigYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected ParseConfigYAML to reject codex key without base-url")
	}
	if !strings.Contains(err.Error(), "codex-api-key") {
		t.Fatalf("expected error mentioning codex-api-key, got %v", err)
	}
}

func TestParseConfigYAMLRejectsLegacyRoutingStrategy(t *testing.T) {
	_, err := ParseConfigYAML([]byte("routing:\n  strategy: fillfirst\n"))
	if err == nil {
		t.Fatal("expected ParseConfigYAML to reject legacy routing strategy")
	}
	if !strings.Contains(err.Error(), "routing.strategy") {
		t.Fatalf("expected routing.strategy error, got %v", err)
	}
}
