package synthesizer

import (
	"testing"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
)

func TestConfigSynthesizer_CodexKeysExposeOnlyRetainedFields(t *testing.T) {
	cfg := &config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:     " config-key ",
			BaseURL:    " https://example.invalid/v1 ",
			Priority:   7,
			Websockets: true,
			Headers: map[string]string{
				" X-Test ": " ok ",
			},
		}},
	}

	auths, err := NewConfigSynthesizer().Synthesize(&SynthesisContext{
		Config:      cfg,
		Now:         time.Unix(1, 0),
		IDGenerator: NewStableIDGenerator(),
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(auths) != 1 {
		t.Fatalf("expected 1 auth entry, got %d", len(auths))
	}

	attrs := auths[0].Attributes
	for key, want := range map[string]string{
		"api_key":       "config-key",
		"auth_kind":     "apikey",
		"base_url":      "https://example.invalid/v1",
		"priority":      "7",
		"websockets":    "true",
		"header:X-Test": "ok",
	} {
		if got := attrs[key]; got != want {
			t.Fatalf("expected attribute %q=%q, got %q", key, want, got)
		}
	}
	if got := attrs["source"]; got == "" {
		t.Fatal("expected synthesized auth to include source attribute")
	}
	if got, want := len(attrs), 7; got != want {
		t.Fatalf("expected only retained attributes, got %d: %#v", got, attrs)
	}
}
