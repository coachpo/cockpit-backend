package cockpit

import (
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/registry"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

func TestRegisterModelsForAuth_DefaultsCodexToProModels(t *testing.T) {
	service := &Service{}
	modelID := firstCodexProModelIDWithPrefix(t, "gpt-5")
	authID := "task7-codex-default-pro"

	GlobalModelRegistry().UnregisterClient(authID)
	t.Cleanup(func() {
		GlobalModelRegistry().UnregisterClient(authID)
	})

	service.registerModelsForAuth(&coreauth.Auth{
		ID:       authID,
		Provider: "codex",
	})

	if !GlobalModelRegistry().ClientSupportsModel(authID, modelID) {
		t.Fatalf("expected default codex plan to register %q", modelID)
	}
}

func TestRegisterModelsForAuth_UsesFreePlanCatalog(t *testing.T) {
	service := &Service{}
	authID := "task7-codex-free-plan"
	modelID := firstCodexModelID(t, registry.GetCodexFreeModels())

	GlobalModelRegistry().UnregisterClient(authID)
	t.Cleanup(func() {
		GlobalModelRegistry().UnregisterClient(authID)
	})

	service.registerModelsForAuth(&coreauth.Auth{
		ID:       authID,
		Provider: "codex",
		Attributes: map[string]string{
			"plan_type": "free",
		},
	})

	if !GlobalModelRegistry().ClientSupportsModel(authID, modelID) {
		t.Fatalf("expected free plan model %q to be registered", modelID)
	}
	if proOnly, ok := firstUniqueCodexModelID(registry.GetCodexProModels(), registry.GetCodexFreeModels()); ok && GlobalModelRegistry().ClientSupportsModel(authID, proOnly) {
		t.Fatalf("did not expect free plan to register pro-only model %q", proOnly)
	}
}

func firstCodexModelID(t *testing.T, models []*ModelInfo) string {
	t.Helper()
	for _, model := range models {
		if model == nil || model.ID == "" {
			continue
		}
		return model.ID
	}
	t.Fatal("expected at least one codex model")
	return ""
}

func firstUniqueCodexModelID(primary, excluded []*ModelInfo) (string, bool) {
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, model := range excluded {
		if model == nil || model.ID == "" {
			continue
		}
		excludedSet[model.ID] = struct{}{}
	}
	for _, model := range primary {
		if model == nil || model.ID == "" {
			continue
		}
		if _, found := excludedSet[model.ID]; !found {
			return model.ID, true
		}
	}
	return "", false
}

func firstCodexProModelIDWithPrefix(t *testing.T, prefix string) string {
	t.Helper()
	for _, model := range registry.GetCodexProModels() {
		if model == nil {
			continue
		}
		if strings.HasPrefix(model.ID, prefix) {
			return model.ID
		}
	}
	t.Fatalf("expected a codex pro model with prefix %q", prefix)
	return ""
}
