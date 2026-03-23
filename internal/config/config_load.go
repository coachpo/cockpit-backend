package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"syscall"

	"gopkg.in/yaml.v3"
)

func LoadConfig(configFile string) (*Config, error) {
	return LoadConfigOptional(configFile, false)
}

func LoadConfigOptional(configFile string, optional bool) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if optional {
			if os.IsNotExist(err) || errors.Is(err, syscall.EISDIR) {
				return &Config{}, nil
			}
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if optional && len(data) == 0 {
		return &Config{}, nil
	}
	var cfg Config
	cfg.Host = ""
	cfg.DisableCooling = false
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err = decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if cfg.MaxRetryCredentials < 0 {
		cfg.MaxRetryCredentials = 0
	}
	cfg.SanitizeCodexKeys()
	for i := range cfg.CodexKey {
		if cfg.CodexKey[i].BaseURL == "" {
			return nil, fmt.Errorf("failed to parse config file: codex-api-key[%d].base-url is required", i)
		}
	}
	cfg.SanitizeCodexHeaderDefaults()
	return &cfg, nil
}
