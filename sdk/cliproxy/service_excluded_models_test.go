package cliproxy

import (
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/registry"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

func TestRegisterModelsForAuth_UsesPreMergedExcludedModelsAttribute(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			OAuthExcludedModels: map[string][]string{
				"codex": {"gpt-5-codex-mini"},
			},
		},
	}
	auth := &coreauth.Auth{
		ID:       "auth-codex",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":       "oauth",
			"excluded_models": "gpt-5.1",
			"plan_type":       "pro",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	models := registry.GetAvailableModelsByProvider("codex")
	if len(models) == 0 {
		t.Fatal("expected codex models to be registered")
	}

	for _, model := range models {
		if model == nil {
			continue
		}
		modelID := strings.TrimSpace(model.ID)
		if strings.EqualFold(modelID, "gpt-5.1") {
			t.Fatalf("expected model %q to be excluded by auth attribute", modelID)
		}
	}
}

func TestRegisterModelsForAuth_IgnoresLegacyGeminiVirtualPrimaryAttribute(t *testing.T) {
	service := &Service{}
	auth := &coreauth.Auth{
		ID:       "auth-codex-legacy-flag",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"gemini_virtual_primary": "true",
			"plan_type":              "pro",
		},
	}

	reg := registry.GetGlobalRegistry()
	reg.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		reg.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	models := reg.GetModelsForClient(auth.ID)
	if len(models) == 0 {
		t.Fatal("expected codex models to stay registered when legacy gemini_virtual_primary is present")
	}
}
