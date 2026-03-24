package diff

import (
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	sdkconfig "github.com/coachpo/cockpit-backend/internal/config"
)

func TestBuildConfigChangeDetails_RetainedSurface(t *testing.T) {
	oldCfg := &config.Config{
		Host:                "0.0.0.0",
		Port:                1000,
		DisableCooling:      false,
		RequestRetry:        1,
		MaxRetryCredentials: 1,
		MaxRetryInterval:    1,
		WebsocketAuth:       false,
		QuotaExceeded:       config.QuotaExceeded{SwitchProject: false},
		CodexKey: []config.CodexKey{{
			APIKey:     "x1",
			BaseURL:    "http://old-base",
			Priority:   1,
			Websockets: false,
			Headers:    map[string]string{"H": "1"},
		}},
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys:                    []string{"key-1"},
			PassthroughHeaders:         false,
			Streaming:                  sdkconfig.StreamingConfig{KeepAliveSeconds: 10, BootstrapRetries: 1},
			NonStreamKeepAliveInterval: 0,
		},
		CodexHeaderDefaults: config.CodexHeaderDefaults{UserAgent: "ua-old", BetaFeatures: "beta-old"},
	}
	newCfg := &config.Config{
		Host:                "127.0.0.1",
		Port:                2000,
		DisableCooling:      true,
		RequestRetry:        2,
		MaxRetryCredentials: 3,
		MaxRetryInterval:    4,
		WebsocketAuth:       true,
		QuotaExceeded:       config.QuotaExceeded{SwitchProject: true},
		CodexKey: []config.CodexKey{{
			APIKey:     "x2",
			BaseURL:    "http://new-base",
			Priority:   5,
			Websockets: true,
			Headers:    map[string]string{"H": "2"},
		}},
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys:                    []string{"key-1", "key-2"},
			PassthroughHeaders:         true,
			Streaming:                  sdkconfig.StreamingConfig{KeepAliveSeconds: 20, BootstrapRetries: 3},
			NonStreamKeepAliveInterval: 5,
		},
		CodexHeaderDefaults: config.CodexHeaderDefaults{UserAgent: "ua-new", BetaFeatures: "beta-new"},
	}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "host: 0.0.0.0 -> 127.0.0.1")
	expectContains(t, changes, "port: 1000 -> 2000")
	expectContains(t, changes, "disable-cooling: false -> true")
	expectContains(t, changes, "request-retry: 1 -> 2")
	expectContains(t, changes, "max-retry-credentials: 1 -> 3")
	expectContains(t, changes, "max-retry-interval: 1 -> 4")
	expectContains(t, changes, "ws-auth: false -> true")
	expectContains(t, changes, "passthrough-headers: false -> true")
	expectContains(t, changes, "streaming.keepalive-seconds: 10 -> 20")
	expectContains(t, changes, "streaming.bootstrap-retries: 1 -> 3")
	expectContains(t, changes, "nonstream-keepalive-interval: 0 -> 5")
	expectContains(t, changes, "quota-exceeded.switch-project: false -> true")
	expectContains(t, changes, "api-keys count: 1 -> 2")
	expectContains(t, changes, "codex[0].base-url: http://old-base -> http://new-base")
	expectContains(t, changes, "codex[0].websockets: false -> true")
	expectContains(t, changes, "codex[0].api-key: updated")
	expectContains(t, changes, "codex[0].priority: 1 -> 5")
	expectContains(t, changes, "codex[0].headers: updated")
	expectContains(t, changes, "codex-header-defaults.user-agent: ua-old -> ua-new")
	expectContains(t, changes, "codex-header-defaults.beta-features: beta-old -> beta-new")
}

func TestBuildConfigChangeDetails_NoChanges(t *testing.T) {
	cfg := &config.Config{Port: 8080}
	if details := BuildConfigChangeDetails(cfg, cfg); len(details) != 0 {
		t.Fatalf("expected no change entries, got %v", details)
	}
}

func TestTrimStrings(t *testing.T) {
	out := trimStrings([]string{" a ", "b", "  c"})
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Fatalf("unexpected trimmed strings: %v", out)
	}
}

func expectContains(t *testing.T, list []string, target string) {
	t.Helper()
	for _, entry := range list {
		if entry == target {
			return
		}
	}
	t.Fatalf("expected list to contain %q, got %#v", target, list)
}
