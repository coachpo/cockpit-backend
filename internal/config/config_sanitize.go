package config

import (
	"strings"
)

func (cfg *Config) SanitizeCodexHeaderDefaults() {
	if cfg == nil {
		return
	}
	cfg.CodexHeaderDefaults.UserAgent = strings.TrimSpace(cfg.CodexHeaderDefaults.UserAgent)
	cfg.CodexHeaderDefaults.BetaFeatures = strings.TrimSpace(cfg.CodexHeaderDefaults.BetaFeatures)
}

func (cfg *Config) SanitizeCodexKeys() {
	if cfg == nil || len(cfg.CodexKey) == 0 {
		return
	}
	for i := range cfg.CodexKey {
		NormalizeCodexKey(&cfg.CodexKey[i])
	}
}

func NormalizeCodexAPIKey(value string) string {
	return strings.TrimSpace(value)
}

func NormalizeCodexKey(entry *CodexKey) {
	if entry == nil {
		return
	}
	entry.APIKey = NormalizeCodexAPIKey(entry.APIKey)
	entry.BaseURL = strings.TrimSpace(entry.BaseURL)
	entry.Headers = NormalizeHeaders(entry.Headers)
}

func NormalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	clean := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		clean[key] = val
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}
