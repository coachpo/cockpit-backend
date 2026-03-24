package config

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseConfigYAML(data []byte) (*Config, error) {
	var cfg Config
	cfg.Host = ""
	cfg.DisableCooling = false
	if len(strings.TrimSpace(string(data))) == 0 {
		return &cfg, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config yaml: %w", err)
	}
	if cfg.MaxRetryCredentials < 0 {
		cfg.MaxRetryCredentials = 0
	}
	if normalizedStrategy, ok := NormalizeRoutingStrategy(cfg.Routing.Strategy); ok {
		cfg.Routing.Strategy = normalizedStrategy
	} else {
		return nil, fmt.Errorf("failed to parse config: routing.strategy must be one of round-robin or fill-first")
	}
	cfg.SanitizeCodexKeys()
	for i := range cfg.CodexKey {
		if cfg.CodexKey[i].BaseURL == "" {
			return nil, fmt.Errorf("failed to parse config: codex-api-key[%d].base-url is required", i)
		}
	}
	cfg.SanitizeCodexHeaderDefaults()
	return &cfg, nil
}
