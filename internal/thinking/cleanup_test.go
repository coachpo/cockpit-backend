package thinking_test

import (
	"bytes"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/thinking"
	_ "github.com/coachpo/cockpit-backend/internal/thinking/provider/codex"
	_ "github.com/coachpo/cockpit-backend/internal/thinking/provider/openai"
	"github.com/tidwall/gjson"
)

func TestApplyThinkingIgnoresUnsupportedSourceProviderConfig(t *testing.T) {
	t.Parallel()

	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":2048}}`)

	result, err := thinking.ApplyThinking(body, "custom-model", "claude", "openai", "openai")
	if err != nil {
		t.Fatalf("ApplyThinking returned error: %v", err)
	}
	if got := gjson.GetBytes(result, "reasoning_effort"); got.Exists() {
		t.Fatalf("expected unsupported source thinking config to be ignored, got reasoning_effort=%q in %s", got.String(), string(result))
	}
	if !bytes.Equal(result, body) {
		t.Fatalf("expected body to pass through unchanged, got %s", string(result))
	}
}

func TestStripThinkingConfigUnsupportedProvidersPassthrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		body     []byte
	}{
		{
			name:     "claude",
			provider: "claude",
			body:     []byte(`{"thinking":{"type":"enabled","budget_tokens":1024},"output_config":{"effort":"high"}}`),
		},
		{
			name:     "gemini-cli",
			provider: "gemini-cli",
			body:     []byte(`{"request":{"generationConfig":{"thinkingConfig":{"thinkingBudget":512}}}}`),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := thinking.StripThinkingConfig(tc.body, tc.provider)
			if !bytes.Equal(result, tc.body) {
				t.Fatalf("expected %s body to pass through unchanged, got %s", tc.provider, string(result))
			}
		})
	}
}
