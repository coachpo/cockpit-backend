package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTask4AdjacentCleanupRemovesDeadFeatureFiles(t *testing.T) {
	for _, rel := range []string{
		"internal/watcher/diff/oauth_excluded.go",
		"internal/watcher/diff/oauth_excluded_test.go",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", rel)); !os.IsNotExist(err) {
			if err == nil {
				t.Fatalf("did not expect %s to exist", rel)
			}
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestTask4AdjacentCleanupRemovesDeadFeaturePaths(t *testing.T) {
	tests := []struct {
		path        string
		bannedSnips []string
	}{
		{
			path: "internal/runtime/executor/proxy_helpers.go",
			bannedSnips: []string{
				"cfg.ProxyURL",
				"Priority 2: Use cfg.ProxyURL",
				"Priority 1: Use auth.ProxyURL",
				"Priority 3: Use RoundTripper from context",
			},
		},
		{
			path: "internal/runtime/executor/codex_websocket_helpers.go",
			bannedSnips: []string{
				"newProxyAwareWebsocketDialer(cfg *config.Config, auth *cliproxyauth.Auth)",
				"_ = cfg",
			},
		},
		{
			path: "internal/watcher/config_reload.go",
			bannedSnips: []string{
				"affectedOAuthProviders",
			},
		},
		{
			path: "internal/watcher/clients.go",
			bannedSnips: []string{
				"affectedOAuthProviders",
				"oauth-excluded-models",
				"openAICompatCount",
				"OpenAI-compat",
			},
		},
		{
			path: "internal/watcher/synthesizer/config.go",
			bannedSnips: []string{
				"OpenAI-compat",
			},
		},
		{
			path: "sdk/cliproxy/auth/conductor_alias.go",
			bannedSnips: []string{
				"apiKeyModelAlias",
				"applyAPIKeyModelAlias",
				"rebuildAPIKeyModelAlias",
			},
		},
		{
			path: "sdk/cliproxy/auth/conductor.go",
			bannedSnips: []string{
				"oauthModelAlias",
			},
		},
		{
			path: "internal/watcher/diff/config_diff.go",
			bannedSnips: []string{
				"formatProxyURL(",
				"OpenAI compatibility providers",
			},
		},
		{
			path: "internal/config/config.go",
			bannedSnips: []string{
				"// LoadConfig reads a YAML configuration file from the given path,",
				"//   - configFile: The path to the YAML configuration file",
			},
		},
		{
			path: "internal/watcher/synthesizer/helpers.go",
			bannedSnips: []string{
				"with the global oauth-excluded-models config for the provider",
				"if authKindKey == \"apikey\"",
				"For OAuth: merge per-account excluded models with global provider-level exclusions",
				"cfg *config.Config",
				"cfg == nil",
			},
		},
		{
			path: "internal/util/provider.go",
			bannedSnips: []string{
				"managing HTTP proxies",
				"cfg: The application configuration containing OpenAI compatibility settings.",
			},
		},
		{
			path: "internal/watcher/diff/AGENTS.md",
			bannedSnips: []string{
				"model_hash.go",
				"models_summary.go",
				"oauth_excluded.go",
				"oauth_model_alias.go",
			},
		},
		{
			path: "sdk/cliproxy/auth/AGENTS.md",
			bannedSnips: []string{
				"oauth_model_alias.go",
				"OAuth model alias and config-model pools are the active alias paths",
			},
		},
		{
			path: "internal/util/AGENTS.md",
			bannedSnips: []string{
				"proxy setup",
				"OpenAI-compat alias helpers",
				"proxy.go",
				"config-facing bridge",
			},
		},
		{
			path: "internal/runtime/executor/AGENTS.md",
			bannedSnips: []string{
				"apply thinking/payload config",
				"shared translation, logging, payload, proxy, and usage helpers",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join("..", "..", tc.path))
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			text := string(contents)
			for _, banned := range tc.bannedSnips {
				if strings.Contains(text, banned) {
					t.Fatalf("did not expect %q in %s", banned, tc.path)
				}
			}
		})
	}
}
