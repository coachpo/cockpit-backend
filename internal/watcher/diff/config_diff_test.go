package diff

import (
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	sdkconfig "github.com/coachpo/cockpit-backend/internal/config"
)

func TestBuildConfigChangeDetails(t *testing.T) {
	oldCfg := &config.Config{
		Port:    8080,
		AuthDir: "/tmp/auth-old",
		RemoteManagement: config.RemoteManagement{
			AllowRemote: false,
			SecretKey:   "old",
		},
		OAuthExcludedModels: map[string][]string{
			"providerA": {"m1"},
		},
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name: "compat-a",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k1"},
				},
				Models: []config.OpenAICompatibilityModel{{Name: "m1"}},
			},
		},
	}

	newCfg := &config.Config{
		Port:    9090,
		AuthDir: "/tmp/auth-new",
		RemoteManagement: config.RemoteManagement{
			AllowRemote: true,
			SecretKey:   "new"},
		OAuthExcludedModels: map[string][]string{
			"providerA": {"m1", "m2"},
			"providerB": {"x"},
		},
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name: "compat-a",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k1"},
				},
				Models: []config.OpenAICompatibilityModel{{Name: "m1"}, {Name: "m2"}},
			},
			{
				Name: "compat-b",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k2"},
				},
			},
		},
	}

	details := BuildConfigChangeDetails(oldCfg, newCfg)

	expectContains(t, details, "port: 8080 -> 9090")
	expectContains(t, details, "auth-dir: /tmp/auth-old -> /tmp/auth-new")
	expectContains(t, details, "remote-management.allow-remote: false -> true")
	expectContains(t, details, "remote-management.secret-key: updated")
	expectContains(t, details, "oauth-excluded-models[providera]: updated (1 -> 2 entries)")
	expectContains(t, details, "oauth-excluded-models[providerb]: added (1 entries)")
	expectContains(t, details, "openai-compatibility:")
	expectContains(t, details, "  provider added: compat-b (api-keys=1, models=0)")
	expectContains(t, details, "  provider updated: compat-a (models 1 -> 2)")
}

func TestBuildConfigChangeDetails_NoChanges(t *testing.T) {
	cfg := &config.Config{
		Port: 8080,
	}
	if details := BuildConfigChangeDetails(cfg, cfg); len(details) != 0 {
		t.Fatalf("expected no change entries, got %v", details)
	}
}

func TestBuildConfigChangeDetails_ModelPrefixes(t *testing.T) {
	oldCfg := &config.Config{
		CodexKey: []config.CodexKey{
			{APIKey: "x1", Prefix: "old-x", BaseURL: "http://x", ProxyURL: "http://xp"},
		},
	}
	newCfg := &config.Config{
		CodexKey: []config.CodexKey{
			{APIKey: "x1", Prefix: "new-x", BaseURL: "http://x", ProxyURL: "http://xp"},
		},
	}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "codex[0].prefix: old-x -> new-x")
}

func TestBuildConfigChangeDetails_NilSafe(t *testing.T) {
	if details := BuildConfigChangeDetails(nil, &config.Config{}); len(details) != 0 {
		t.Fatalf("expected empty change list when old nil, got %v", details)
	}
	if details := BuildConfigChangeDetails(&config.Config{}, nil); len(details) != 0 {
		t.Fatalf("expected empty change list when new nil, got %v", details)
	}
}

func TestBuildConfigChangeDetails_SecretsAndCounts(t *testing.T) {
	oldCfg := &config.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"a"},
		},
		RemoteManagement: config.RemoteManagement{
			SecretKey: "",
		},
	}
	newCfg := &config.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"a", "b", "c"},
		},
		RemoteManagement: config.RemoteManagement{
			SecretKey: "new-secret",
		},
	}

	details := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, details, "api-keys count: 1 -> 3")
	expectContains(t, details, "remote-management.secret-key: created")
}

func TestBuildConfigChangeDetails_FlagsAndKeys(t *testing.T) {
	oldCfg := &config.Config{
		Port:                1000,
		AuthDir:             "/old",
		Debug:               false,
		DisableCooling:      false,
		RequestRetry:        1,
		MaxRetryCredentials: 1,
		MaxRetryInterval:    1,
		WebsocketAuth:       false,
		QuotaExceeded:       config.QuotaExceeded{SwitchProject: false, SwitchPreviewModel: false},
		CodexKey:            []config.CodexKey{{APIKey: "x1"}},
		RemoteManagement:    config.RemoteManagement{SecretKey: "keep"},
		SDKConfig: sdkconfig.SDKConfig{
			RequestLog:                 false,
			ProxyURL:                   "http://old-proxy",
			APIKeys:                    []string{"key-1"},
			ForceModelPrefix:           false,
			NonStreamKeepAliveInterval: 0,
		},
	}
	newCfg := &config.Config{
		Port:                2000,
		AuthDir:             "/new",
		Debug:               true,
		DisableCooling:      true,
		RequestRetry:        2,
		MaxRetryCredentials: 3,
		MaxRetryInterval:    3,
		WebsocketAuth:       true,
		QuotaExceeded:       config.QuotaExceeded{SwitchProject: true, SwitchPreviewModel: true},
		CodexKey: []config.CodexKey{
			{APIKey: "x1", BaseURL: "http://x", ProxyURL: "http://px", Headers: map[string]string{"H": "2"}, ExcludedModels: []string{"b"}},
			{APIKey: "x2"},
		},
		RemoteManagement: config.RemoteManagement{SecretKey: ""},
		SDKConfig: sdkconfig.SDKConfig{
			RequestLog:                 true,
			ProxyURL:                   "http://new-proxy",
			APIKeys:                    []string{" key-1 ", "key-2"},
			ForceModelPrefix:           true,
			NonStreamKeepAliveInterval: 5,
		},
	}

	details := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, details, "debug: false -> true")
	expectContains(t, details, "disable-cooling: false -> true")
	expectContains(t, details, "request-log: false -> true")
	expectContains(t, details, "request-retry: 1 -> 2")
	expectContains(t, details, "max-retry-credentials: 1 -> 3")
	expectContains(t, details, "max-retry-interval: 1 -> 3")
	expectContains(t, details, "proxy-url: http://old-proxy -> http://new-proxy")
	expectContains(t, details, "ws-auth: false -> true")
	expectContains(t, details, "force-model-prefix: false -> true")
	expectContains(t, details, "nonstream-keepalive-interval: 0 -> 5")
	expectContains(t, details, "quota-exceeded.switch-project: false -> true")
	expectContains(t, details, "quota-exceeded.switch-preview-model: false -> true")
	expectContains(t, details, "api-keys count: 1 -> 2")
	expectContains(t, details, "codex-api-key count: 1 -> 2")
	expectContains(t, details, "remote-management.secret-key: deleted")
}

func TestBuildConfigChangeDetails_AllBranches(t *testing.T) {
	oldCfg := &config.Config{
		Port:                1,
		AuthDir:             "/a",
		Debug:               false,
		DisableCooling:      false,
		RequestRetry:        1,
		MaxRetryCredentials: 1,
		MaxRetryInterval:    1,
		WebsocketAuth:       false,
		QuotaExceeded:       config.QuotaExceeded{SwitchProject: false, SwitchPreviewModel: false},
		CodexKey: []config.CodexKey{
			{APIKey: "x-old", BaseURL: "http://x-old", ProxyURL: "http://xp-old", Headers: map[string]string{"H": "1"}, ExcludedModels: []string{"x"}},
		},
		RemoteManagement: config.RemoteManagement{
			AllowRemote: false, SecretKey: "old",
		},
		SDKConfig: sdkconfig.SDKConfig{
			RequestLog: false,
			ProxyURL:   "http://old-proxy",
			APIKeys:    []string{" keyA "},
		},
		OAuthExcludedModels: map[string][]string{"p1": {"a"}},
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name: "prov-old",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k1"},
				},
				Models: []config.OpenAICompatibilityModel{{Name: "m1"}},
			},
		},
	}
	newCfg := &config.Config{
		Port:                2,
		AuthDir:             "/b",
		Debug:               true,
		DisableCooling:      true,
		RequestRetry:        2,
		MaxRetryCredentials: 3,
		MaxRetryInterval:    3,
		WebsocketAuth:       true,
		QuotaExceeded:       config.QuotaExceeded{SwitchProject: true, SwitchPreviewModel: true},
		CodexKey: []config.CodexKey{
			{APIKey: "x-new", BaseURL: "http://x-new", ProxyURL: "http://xp-new", Headers: map[string]string{"H": "2"}, ExcludedModels: []string{"x", "y"}},
		},
		RemoteManagement: config.RemoteManagement{
			AllowRemote: true, SecretKey: "",
		},
		SDKConfig: sdkconfig.SDKConfig{
			RequestLog: true,
			ProxyURL:   "http://new-proxy",
			APIKeys:    []string{"keyB"},
		},
		OAuthExcludedModels: map[string][]string{"p1": {"b", "c"}, "p2": {"d"}},
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name: "prov-old",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k1"},
					{APIKey: "k2"},
				},
				Models: []config.OpenAICompatibilityModel{{Name: "m1"}, {Name: "m2"}},
			},
			{
				Name:          "prov-new",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "k3"}},
			},
		},
	}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "port: 1 -> 2")
	expectContains(t, changes, "auth-dir: /a -> /b")
	expectContains(t, changes, "debug: false -> true")
	expectContains(t, changes, "disable-cooling: false -> true")
	expectContains(t, changes, "request-retry: 1 -> 2")
	expectContains(t, changes, "max-retry-credentials: 1 -> 3")
	expectContains(t, changes, "max-retry-interval: 1 -> 3")
	expectContains(t, changes, "proxy-url: http://old-proxy -> http://new-proxy")
	expectContains(t, changes, "ws-auth: false -> true")
	expectContains(t, changes, "quota-exceeded.switch-project: false -> true")
	expectContains(t, changes, "quota-exceeded.switch-preview-model: false -> true")
	expectContains(t, changes, "api-keys: values updated (count unchanged, redacted)")
	expectContains(t, changes, "codex[0].base-url: http://x-old -> http://x-new")
	expectContains(t, changes, "codex[0].proxy-url: http://xp-old -> http://xp-new")
	expectContains(t, changes, "codex[0].api-key: updated")
	expectContains(t, changes, "codex[0].headers: updated")
	expectContains(t, changes, "codex[0].excluded-models: updated (1 -> 2 entries)")
	expectContains(t, changes, "oauth-excluded-models[p1]: updated (1 -> 2 entries)")
	expectContains(t, changes, "oauth-excluded-models[p2]: added (1 entries)")
	expectContains(t, changes, "remote-management.allow-remote: false -> true")
	expectContains(t, changes, "remote-management.secret-key: deleted")
	expectContains(t, changes, "openai-compatibility:")
}

func TestFormatProxyURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "<none>"},
		{name: "invalid", in: "http://[::1", want: "<redacted>"},
		{name: "fullURLRedactsUserinfoAndPath", in: "http://user:pass@example.com:8080/path?x=1#frag", want: "http://example.com:8080"},
		{name: "socks5RedactsUserinfoAndPath", in: "socks5://user:pass@192.168.1.1:1080/path?x=1", want: "socks5://192.168.1.1:1080"},
		{name: "socks5HostPort", in: "socks5://proxy.example.com:1080/", want: "socks5://proxy.example.com:1080"},
		{name: "hostPortNoScheme", in: "example.com:1234/path?x=1", want: "example.com:1234"},
		{name: "relativePathRedacted", in: "/just/path", want: "<redacted>"},
		{name: "schemeAndHost", in: "https://example.com", want: "https://example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatProxyURL(tt.in); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildConfigChangeDetails_SecretUpdates(t *testing.T) {
	oldCfg := &config.Config{
		RemoteManagement: config.RemoteManagement{
			SecretKey: "old",
		},
	}
	newCfg := &config.Config{
		RemoteManagement: config.RemoteManagement{
			SecretKey: "new",
		},
	}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "remote-management.secret-key: updated")
}

func TestBuildConfigChangeDetails_CountBranches(t *testing.T) {
	oldCfg := &config.Config{}
	newCfg := &config.Config{
		CodexKey: []config.CodexKey{{APIKey: "x"}},
	}

	changes := BuildConfigChangeDetails(oldCfg, newCfg)
	expectContains(t, changes, "codex-api-key count: 0 -> 1")
}

func TestTrimStrings(t *testing.T) {
	out := trimStrings([]string{" a ", "b", "  c"})
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Fatalf("unexpected trimmed strings: %v", out)
	}
}
