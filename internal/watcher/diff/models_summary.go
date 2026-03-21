package diff

import (
	"strings"

	"github.com/coachpo/cockpit-backend/internal/config"
)

type CodexModelsSummary struct {
	hash  string
	count int
}

// SummarizeCodexModels hashes Codex model aliases for change detection.
func SummarizeCodexModels(models []config.CodexModel) CodexModelsSummary {
	if len(models) == 0 {
		return CodexModelsSummary{}
	}
	keys := normalizeModelPairs(func(out func(key string)) {
		for _, model := range models {
			name := strings.TrimSpace(model.Name)
			alias := strings.TrimSpace(model.Alias)
			if name == "" && alias == "" {
				continue
			}
			out(strings.ToLower(name) + "|" + strings.ToLower(alias))
		}
	})
	return CodexModelsSummary{
		hash:  hashJoined(keys),
		count: len(keys),
	}
}
