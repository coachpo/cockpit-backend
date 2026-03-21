package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/coachpo/cockpit-backend/internal/translator"

	// Import provider packages to trigger init() registration of ProviderAppliers
	_ "github.com/coachpo/cockpit-backend/internal/thinking/provider/codex"
	_ "github.com/coachpo/cockpit-backend/internal/thinking/provider/openai"

	"github.com/coachpo/cockpit-backend/internal/registry"
	"github.com/coachpo/cockpit-backend/internal/thinking"
	sdktranslator "github.com/coachpo/cockpit-backend/sdk/translator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// thinkingTestCase represents a common test case structure for both suffix and body tests.
type thinkingTestCase struct {
	name            string
	from            string
	to              string
	model           string
	inputJSON       string
	expectField     string
	expectValue     string
	expectField2    string
	expectValue2    string
	includeThoughts string
	expectErr       bool
}

// TestThinkingE2EMatrix_Suffix tests the thinking configuration transformation using model name suffix.
// Data flow: Input JSON → TranslateRequest → ApplyThinking → Validate Output
// No helper functions are used; all test data is inline.
func TestThinkingE2EMatrix_Suffix(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	uid := fmt.Sprintf("thinking-e2e-suffix-%d", time.Now().UnixNano())

	reg.RegisterClient(uid, "test", getTestModels())
	defer reg.UnregisterClient(uid)

	cases := []thinkingTestCase{
		// level-model (Levels=minimal/low/medium/high, ZeroAllowed=false, DynamicAllowed=false)

		// Case 1: No suffix → injected default → medium
		{
			name:        "1",
			from:        "openai",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning.effort",
			expectValue: "medium",
			expectErr:   false,
		},
		// Case 2: Specified medium → medium
		{
			name:        "2",
			from:        "openai",
			to:          "codex",
			model:       "level-model(medium)",
			inputJSON:   `{"model":"level-model(medium)","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning.effort",
			expectValue: "medium",
			expectErr:   false,
		},
		// Case 3: Specified xhigh → out of range error
		{
			name:        "3",
			from:        "openai",
			to:          "codex",
			model:       "level-model(xhigh)",
			inputJSON:   `{"model":"level-model(xhigh)","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "",
			expectErr:   true,
		},
		// Case 4: Level none → clamped to minimal (ZeroAllowed=false)
		{
			name:        "4",
			from:        "openai",
			to:          "codex",
			model:       "level-model(none)",
			inputJSON:   `{"model":"level-model(none)","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning.effort",
			expectValue: "minimal",
			expectErr:   false,
		},
		// Case 5: Level auto → DynamicAllowed=false → medium (mid-range)
		{
			name:        "5",
			from:        "openai",
			to:          "codex",
			model:       "level-model(auto)",
			inputJSON:   `{"model":"level-model(auto)","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning.effort",
			expectValue: "medium",
			expectErr:   false,
		},

		// Same-protocol passthrough tests (80-83)

		// Case 80: OpenAI to OpenAI, level high → passthrough reasoning_effort
		{
			name:        "80",
			from:        "openai",
			to:          "openai",
			model:       "level-model(high)",
			inputJSON:   `{"model":"level-model(high)","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning_effort",
			expectValue: "high",
			expectErr:   false,
		},
		// Case 81: OpenAI to OpenAI, level xhigh → out of range error
		{
			name:        "81",
			from:        "openai",
			to:          "openai",
			model:       "level-model(xhigh)",
			inputJSON:   `{"model":"level-model(xhigh)","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "",
			expectErr:   true,
		},
		// Case 82: OpenAI-Response to Codex, level high → passthrough reasoning.effort
		{
			name:        "82",
			from:        "openai-response",
			to:          "codex",
			model:       "level-model(high)",
			inputJSON:   `{"model":"level-model(high)","input":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning.effort",
			expectValue: "high",
			expectErr:   false,
		},
		// Case 83: OpenAI-Response to Codex, level xhigh → out of range error
		{
			name:        "83",
			from:        "openai-response",
			to:          "codex",
			model:       "level-model(xhigh)",
			inputJSON:   `{"model":"level-model(xhigh)","input":[{"role":"user","content":"hi"}]}`,
			expectField: "",
			expectErr:   true,
		},
	}

	runThinkingTests(t, cases)
}

// TestThinkingE2EMatrix_Body tests the thinking configuration transformation using request body parameters.
// Data flow: Input JSON with thinking params → TranslateRequest → ApplyThinking → Validate Output
func TestThinkingE2EMatrix_Body(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	uid := fmt.Sprintf("thinking-e2e-body-%d", time.Now().UnixNano())

	reg.RegisterClient(uid, "test", getTestModels())
	defer reg.UnregisterClient(uid)

	cases := []thinkingTestCase{
		// level-model (Levels=minimal/low/medium/high, ZeroAllowed=false, DynamicAllowed=false)

		// Case 1: No param → injected default → medium
		{
			name:        "1",
			from:        "openai",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}]}`,
			expectField: "reasoning.effort",
			expectValue: "medium",
			expectErr:   false,
		},
		// Case 2: reasoning_effort=medium → medium
		{
			name:        "2",
			from:        "openai",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"medium"}`,
			expectField: "reasoning.effort",
			expectValue: "medium",
			expectErr:   false,
		},
		// Case 3: reasoning_effort=xhigh → out of range error
		{
			name:        "3",
			from:        "openai",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"xhigh"}`,
			expectField: "",
			expectErr:   true,
		},
		// Case 4: reasoning_effort=none → clamped to minimal
		{
			name:        "4",
			from:        "openai",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"none"}`,
			expectField: "reasoning.effort",
			expectValue: "minimal",
			expectErr:   false,
		},
		// Case 5: reasoning_effort=auto → medium (DynamicAllowed=false)
		{
			name:        "5",
			from:        "openai",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"auto"}`,
			expectField: "reasoning.effort",
			expectValue: "medium",
			expectErr:   false,
		},

		// Same-protocol passthrough tests (80-83)

		// Case 80: OpenAI to OpenAI, reasoning_effort=high → passthrough
		{
			name:        "80",
			from:        "openai",
			to:          "openai",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			expectField: "reasoning_effort",
			expectValue: "high",
			expectErr:   false,
		},
		// Case 81: OpenAI to OpenAI, reasoning_effort=xhigh → out of range error
		{
			name:        "81",
			from:        "openai",
			to:          "openai",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"xhigh"}`,
			expectField: "",
			expectErr:   true,
		},
		// Case 82: OpenAI-Response to Codex, reasoning.effort=high → passthrough
		{
			name:        "82",
			from:        "openai-response",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","input":[{"role":"user","content":"hi"}],"reasoning":{"effort":"high"}}`,
			expectField: "reasoning.effort",
			expectValue: "high",
			expectErr:   false,
		},
		// Case 83: OpenAI-Response to Codex, reasoning.effort=xhigh → out of range error
		{
			name:        "83",
			from:        "openai-response",
			to:          "codex",
			model:       "level-model",
			inputJSON:   `{"model":"level-model","input":[{"role":"user","content":"hi"}],"reasoning":{"effort":"xhigh"}}`,
			expectField: "",
			expectErr:   true,
		},
	}

	runThinkingTests(t, cases)
}

// getTestModels returns the shared model definitions for E2E tests.
func getTestModels() []*registry.ModelInfo {
	return []*registry.ModelInfo{
		{
			ID:          "level-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "openai",
			DisplayName: "Level Model",
			Thinking:    &registry.ThinkingSupport{Levels: []string{"minimal", "low", "medium", "high"}, ZeroAllowed: false, DynamicAllowed: false},
		},
		{
			ID:          "level-subset-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "gemini",
			DisplayName: "Level Subset Model",
			Thinking:    &registry.ThinkingSupport{Levels: []string{"low", "high"}, ZeroAllowed: false, DynamicAllowed: false},
		},
		{
			ID:          "gemini-budget-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "gemini",
			DisplayName: "Gemini Budget Model",
			Thinking:    &registry.ThinkingSupport{Min: 128, Max: 20000, ZeroAllowed: false, DynamicAllowed: true},
		},
		{
			ID:          "gemini-mixed-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "gemini",
			DisplayName: "Gemini Mixed Model",
			Thinking:    &registry.ThinkingSupport{Min: 128, Max: 32768, Levels: []string{"low", "high"}, ZeroAllowed: false, DynamicAllowed: true},
		},
		{
			ID:          "claude-budget-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "claude",
			DisplayName: "Claude Budget Model",
			Thinking:    &registry.ThinkingSupport{Min: 1024, Max: 128000, ZeroAllowed: true, DynamicAllowed: false},
		},
		{
			ID:                  "claude-sonnet-4-6-model",
			Object:              "model",
			Created:             1771372800, // 2026-02-17
			OwnedBy:             "anthropic",
			Type:                "claude",
			DisplayName:         "Claude 4.6 Sonnet",
			ContextLength:       200000,
			MaxCompletionTokens: 64000,
			Thinking:            &registry.ThinkingSupport{Min: 1024, Max: 128000, ZeroAllowed: true, DynamicAllowed: false, Levels: []string{"low", "medium", "high"}},
		},
		{
			ID:                  "claude-opus-4-6-model",
			Object:              "model",
			Created:             1770318000, // 2026-02-05
			OwnedBy:             "anthropic",
			Type:                "claude",
			DisplayName:         "Claude 4.6 Opus",
			Description:         "Premium model combining maximum intelligence with practical performance",
			ContextLength:       1000000,
			MaxCompletionTokens: 128000,
			Thinking:            &registry.ThinkingSupport{Min: 1024, Max: 128000, ZeroAllowed: true, DynamicAllowed: false, Levels: []string{"low", "medium", "high", "max"}},
		},
		{
			ID:          "gemini-cli-budget-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "gemini-cli",
			DisplayName: "Gemini CLI Budget Model",
			Thinking:    &registry.ThinkingSupport{Min: 128, Max: 20000, ZeroAllowed: true, DynamicAllowed: true},
		},
		{
			ID:          "no-thinking-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "openai",
			DisplayName: "No Thinking Model",
			Thinking:    nil,
		},
		{
			ID:          "user-defined-model",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "openai",
			DisplayName: "User Defined Model",
			UserDefined: true,
			Thinking:    nil,
		},
		{
			ID:          "glm-test",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "iflow",
			DisplayName: "GLM Test Model",
			Thinking:    &registry.ThinkingSupport{Levels: []string{"none", "auto", "minimal", "low", "medium", "high", "xhigh"}},
		},
		{
			ID:          "minimax-test",
			Object:      "model",
			Created:     1700000000,
			OwnedBy:     "test",
			Type:        "iflow",
			DisplayName: "MiniMax Test Model",
			Thinking:    &registry.ThinkingSupport{Levels: []string{"none", "auto", "minimal", "low", "medium", "high", "xhigh"}},
		},
	}
}

// runThinkingTests runs thinking test cases using the real data flow path.
func runThinkingTests(t *testing.T, cases []thinkingTestCase) {
	for _, tc := range cases {
		tc := tc
		testName := fmt.Sprintf("Case%s_%s->%s_%s", tc.name, tc.from, tc.to, tc.model)
		t.Run(testName, func(t *testing.T) {
			suffixResult := thinking.ParseSuffix(tc.model)
			baseModel := suffixResult.ModelName

			translateTo := tc.to
			applyTo := tc.to
			if tc.to == "iflow" {
				translateTo = "openai"
				applyTo = "iflow"
			}

			body := sdktranslator.TranslateRequest(
				sdktranslator.FromString(tc.from),
				sdktranslator.FromString(translateTo),
				baseModel,
				[]byte(tc.inputJSON),
				true,
			)
			if applyTo == "claude" {
				body, _ = sjson.SetBytes(body, "max_tokens", 200000)
			}

			body, err := thinking.ApplyThinking(body, tc.model, tc.from, applyTo, applyTo)

			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error but got none, body=%s", string(body))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v, body=%s", err, string(body))
			}

			if tc.expectField == "" {
				var hasThinking bool
				switch tc.to {
				case "gemini":
					hasThinking = gjson.GetBytes(body, "generationConfig.thinkingConfig").Exists()
				case "gemini-cli":
					hasThinking = gjson.GetBytes(body, "request.generationConfig.thinkingConfig").Exists()
				case "claude":
					hasThinking = gjson.GetBytes(body, "thinking").Exists()
				case "openai":
					hasThinking = gjson.GetBytes(body, "reasoning_effort").Exists()
				case "codex":
					hasThinking = gjson.GetBytes(body, "reasoning.effort").Exists() || gjson.GetBytes(body, "reasoning").Exists()
				case "iflow":
					hasThinking = gjson.GetBytes(body, "chat_template_kwargs.enable_thinking").Exists() || gjson.GetBytes(body, "reasoning_split").Exists()
				}
				if hasThinking {
					t.Fatalf("expected no thinking field but found one, body=%s", string(body))
				}
				return
			}

			assertField := func(fieldPath, expected string) {
				val := gjson.GetBytes(body, fieldPath)
				if !val.Exists() {
					t.Fatalf("expected field %s not found, body=%s", fieldPath, string(body))
				}
				actualValue := val.String()
				if val.Type == gjson.Number {
					actualValue = fmt.Sprintf("%d", val.Int())
				}
				if actualValue != expected {
					t.Fatalf("field %s: expected %q, got %q, body=%s", fieldPath, expected, actualValue, string(body))
				}
			}

			assertField(tc.expectField, tc.expectValue)
			if tc.expectField2 != "" {
				assertField(tc.expectField2, tc.expectValue2)
			}

			if tc.includeThoughts != "" && (tc.to == "gemini" || tc.to == "gemini-cli") {
				path := "generationConfig.thinkingConfig.includeThoughts"
				if tc.to == "gemini-cli" {
					path = "request.generationConfig.thinkingConfig.includeThoughts"
				}
				itVal := gjson.GetBytes(body, path)
				if !itVal.Exists() {
					t.Fatalf("expected includeThoughts field not found, body=%s", string(body))
				}
				actual := fmt.Sprintf("%v", itVal.Bool())
				if actual != tc.includeThoughts {
					t.Fatalf("includeThoughts: expected %s, got %s, body=%s", tc.includeThoughts, actual, string(body))
				}
			}

			// Verify clear_thinking for iFlow GLM models when enable_thinking=true
			if tc.to == "iflow" && tc.expectField == "chat_template_kwargs.enable_thinking" && tc.expectValue == "true" {
				baseModel := thinking.ParseSuffix(tc.model).ModelName
				isGLM := strings.HasPrefix(strings.ToLower(baseModel), "glm")
				ctVal := gjson.GetBytes(body, "chat_template_kwargs.clear_thinking")
				if isGLM {
					if !ctVal.Exists() {
						t.Fatalf("expected clear_thinking field not found for GLM model, body=%s", string(body))
					}
					if ctVal.Bool() != false {
						t.Fatalf("clear_thinking: expected false, got %v, body=%s", ctVal.Bool(), string(body))
					}
				} else if ctVal.Exists() {
					t.Fatalf("expected no clear_thinking field for non-GLM enable_thinking model, body=%s", string(body))
				}
			}
		})
	}
}
