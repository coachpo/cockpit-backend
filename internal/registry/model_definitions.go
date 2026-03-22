// Package registry provides model definitions and lookup helpers for various AI providers.
// Static model metadata is loaded from the embedded models.json file and can be refreshed from network.
package registry

import (
	"strings"
)

// staticModelsJSON mirrors the top-level structure of models.json.
type staticModelsJSON struct {
	CodexFree []*ModelInfo `json:"codex-free"`
	CodexTeam []*ModelInfo `json:"codex-team"`
	CodexPlus []*ModelInfo `json:"codex-plus"`
	CodexPro  []*ModelInfo `json:"codex-pro"`
}

// GetCodexFreeModels returns model definitions for the Codex free plan tier.
func GetCodexFreeModels() []*ModelInfo {
	return codexModelsWithKnownAdditions(getModels().CodexFree)
}

// GetCodexTeamModels returns model definitions for the Codex team plan tier.
func GetCodexTeamModels() []*ModelInfo {
	return codexModelsWithKnownAdditions(getModels().CodexTeam)
}

// GetCodexPlusModels returns model definitions for the Codex plus plan tier.
func GetCodexPlusModels() []*ModelInfo {
	return codexModelsWithKnownAdditions(getModels().CodexPlus)
}

// GetCodexProModels returns model definitions for the Codex pro plan tier.
func GetCodexProModels() []*ModelInfo {
	return codexModelsWithKnownAdditions(getModels().CodexPro)
}

func codexModelsWithKnownAdditions(models []*ModelInfo) []*ModelInfo {
	cloned := cloneModelInfos(models)
	return ensureKnownCodexMiniModel(cloned)
}

func ensureKnownCodexMiniModel(models []*ModelInfo) []*ModelInfo {
	if len(models) == 0 {
		return models
	}
	for _, model := range models {
		if model != nil && model.ID == "gpt-5.4-mini" {
			return models
		}
	}
	var template *ModelInfo
	for _, model := range models {
		if model != nil && model.ID == "gpt-5.4" {
			template = cloneModelInfo(model)
			break
		}
	}
	if template == nil {
		return models
	}
	template.ID = "gpt-5.4-mini"
	if strings.TrimSpace(template.DisplayName) != "" {
		template.DisplayName = "GPT-5.4 mini"
	}
	if strings.TrimSpace(template.Name) != "" {
		template.Name = "gpt-5.4-mini"
	}
	if strings.TrimSpace(template.Version) != "" {
		template.Version = "gpt-5.4-mini"
	}
	return append(models, template)
}

// cloneModelInfos returns a shallow copy of the slice with each element deep-cloned.
func cloneModelInfos(models []*ModelInfo) []*ModelInfo {
	if len(models) == 0 {
		return nil
	}
	out := make([]*ModelInfo, len(models))
	for i, m := range models {
		out[i] = cloneModelInfo(m)
	}
	return out
}

// GetStaticModelDefinitionsByChannel returns static model definitions for a given channel/provider.
// It returns nil when the channel is unknown.
//
// Supported channels:
//   - codex
func GetStaticModelDefinitionsByChannel(channel string) []*ModelInfo {
	key := strings.ToLower(strings.TrimSpace(channel))
	switch key {
	case "codex":
		return GetCodexProModels()
	default:
		return nil
	}
}

// LookupStaticModelInfo searches all static model definitions for a model by ID.
// Returns nil if no matching model is found.
func LookupStaticModelInfo(modelID string) *ModelInfo {
	if modelID == "" {
		return nil
	}

	allModels := [][]*ModelInfo{
		GetCodexFreeModels(),
		GetCodexTeamModels(),
		GetCodexPlusModels(),
		GetCodexProModels(),
	}
	for _, models := range allModels {
		for _, m := range models {
			if m != nil && m.ID == modelID {
				return cloneModelInfo(m)
			}
		}
	}

	return nil
}
