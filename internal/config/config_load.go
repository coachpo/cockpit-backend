package config

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/crypto/bcrypt"
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
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		if optional {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if cfg.RemoteManagement.SecretKey != "" && !looksLikeBcrypt(cfg.RemoteManagement.SecretKey) {
		hashed, errHash := hashSecret(cfg.RemoteManagement.SecretKey)
		if errHash != nil {
			return nil, fmt.Errorf("failed to hash remote management key: %w", errHash)
		}
		cfg.RemoteManagement.SecretKey = hashed
	}
	if cfg.MaxRetryCredentials < 0 {
		cfg.MaxRetryCredentials = 0
	}
	cfg.SanitizeCodexKeys()
	cfg.SanitizeCodexHeaderDefaults()
	cfg.SanitizeOpenAICompatibility()
	cfg.OAuthExcludedModels = NormalizeOAuthExcludedModels(cfg.OAuthExcludedModels)
	cfg.SanitizeOAuthModelAlias()
	cfg.SanitizePayloadRules()
	return &cfg, nil
}

func looksLikeBcrypt(s string) bool {
	return len(s) > 4 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

func hashSecret(secret string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}
