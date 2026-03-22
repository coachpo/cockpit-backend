package config

import "testing"

func TestSanitizeCodexKeysNormalizesRetainedFieldsWithoutDroppingEntries(t *testing.T) {
	cfg := &Config{
		CodexKey: []CodexKey{
			{APIKey: "missing-base", BaseURL: "   ", Priority: 9, Websockets: true},
			{APIKey: "kept", BaseURL: " https://example.invalid/v1 ", Priority: 7, Websockets: true, Headers: map[string]string{" X-Test ": " ok ", "Drop": "   "}},
		},
	}

	cfg.SanitizeCodexKeys()

	if len(cfg.CodexKey) != 2 {
		t.Fatalf("expected codex entries to be preserved during sanitize, got %d", len(cfg.CodexKey))
	}
	if cfg.CodexKey[0].APIKey != "missing-base" {
		t.Fatalf("expected first api-key to be preserved after trim, got %q", cfg.CodexKey[0].APIKey)
	}
	if cfg.CodexKey[0].BaseURL != "" {
		t.Fatalf("expected blank base-url to stay blank after normalize, got %q", cfg.CodexKey[0].BaseURL)
	}
	if cfg.CodexKey[1].APIKey != "kept" {
		t.Fatalf("expected second codex key to be trimmed, got %q", cfg.CodexKey[1].APIKey)
	}
	if cfg.CodexKey[1].BaseURL != "https://example.invalid/v1" {
		t.Fatalf("expected base-url to be trimmed, got %q", cfg.CodexKey[1].BaseURL)
	}
	if cfg.CodexKey[1].Priority != 7 {
		t.Fatalf("expected priority to be preserved, got %d", cfg.CodexKey[1].Priority)
	}
	if !cfg.CodexKey[1].Websockets {
		t.Fatal("expected websockets to be preserved")
	}
	if cfg.CodexKey[1].Headers["X-Test"] != "ok" {
		t.Fatalf("expected headers to be normalized, got %#v", cfg.CodexKey[1].Headers)
	}
	if len(cfg.CodexKey[1].Headers) != 1 {
		t.Fatalf("expected only normalized header to remain, got %#v", cfg.CodexKey[1].Headers)
	}
	if _, ok := cfg.CodexKey[1].Headers[" X-Test "]; ok {
		t.Fatalf("expected untrimmed header key to be removed, got %#v", cfg.CodexKey[1].Headers)
	}
	if _, ok := cfg.CodexKey[1].Headers["X-Test"]; !ok {
		t.Fatalf("expected normalized header key to exist, got %#v", cfg.CodexKey[1].Headers)
	}
}

func TestSanitizeCodexKeysTrimsAPIKey(t *testing.T) {
	cfg := &Config{
		CodexKey: []CodexKey{{
			APIKey:  " trimmed-key ",
			BaseURL: "https://example.invalid/v1",
		}},
	}

	cfg.SanitizeCodexKeys()

	if got := cfg.CodexKey[0].APIKey; got != "trimmed-key" {
		t.Fatalf("expected api-key to be trimmed during sanitize, got %q", got)
	}
}
