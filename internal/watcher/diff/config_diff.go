package diff

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/coachpo/cockpit-backend/internal/config"
)

// BuildConfigChangeDetails computes a redacted, human-readable list of config changes.
// Secrets are never printed; only structural or non-sensitive fields are surfaced.
func BuildConfigChangeDetails(oldCfg, newCfg *config.Config) []string {
	changes := make([]string, 0, 16)
	if oldCfg == nil || newCfg == nil {
		return changes
	}

	if oldCfg.Host != newCfg.Host {
		changes = append(changes, fmt.Sprintf("host: %s -> %s", oldCfg.Host, newCfg.Host))
	}

	if oldCfg.Port != newCfg.Port {
		changes = append(changes, fmt.Sprintf("port: %d -> %d", oldCfg.Port, newCfg.Port))
	}
	if oldCfg.DisableCooling != newCfg.DisableCooling {
		changes = append(changes, fmt.Sprintf("disable-cooling: %t -> %t", oldCfg.DisableCooling, newCfg.DisableCooling))
	}
	if oldCfg.RequestRetry != newCfg.RequestRetry {
		changes = append(changes, fmt.Sprintf("request-retry: %d -> %d", oldCfg.RequestRetry, newCfg.RequestRetry))
	}
	if oldCfg.MaxRetryCredentials != newCfg.MaxRetryCredentials {
		changes = append(changes, fmt.Sprintf("max-retry-credentials: %d -> %d", oldCfg.MaxRetryCredentials, newCfg.MaxRetryCredentials))
	}
	if oldCfg.MaxRetryInterval != newCfg.MaxRetryInterval {
		changes = append(changes, fmt.Sprintf("max-retry-interval: %d -> %d", oldCfg.MaxRetryInterval, newCfg.MaxRetryInterval))
	}
	if oldCfg.WebsocketAuth != newCfg.WebsocketAuth {
		changes = append(changes, fmt.Sprintf("ws-auth: %t -> %t", oldCfg.WebsocketAuth, newCfg.WebsocketAuth))
	}
	if oldCfg.PassthroughHeaders != newCfg.PassthroughHeaders {
		changes = append(changes, fmt.Sprintf("passthrough-headers: %t -> %t", oldCfg.PassthroughHeaders, newCfg.PassthroughHeaders))
	}
	if oldCfg.Streaming.KeepAliveSeconds != newCfg.Streaming.KeepAliveSeconds {
		changes = append(changes, fmt.Sprintf("streaming.keepalive-seconds: %d -> %d", oldCfg.Streaming.KeepAliveSeconds, newCfg.Streaming.KeepAliveSeconds))
	}
	if oldCfg.Streaming.BootstrapRetries != newCfg.Streaming.BootstrapRetries {
		changes = append(changes, fmt.Sprintf("streaming.bootstrap-retries: %d -> %d", oldCfg.Streaming.BootstrapRetries, newCfg.Streaming.BootstrapRetries))
	}
	if oldCfg.NonStreamKeepAliveInterval != newCfg.NonStreamKeepAliveInterval {
		changes = append(changes, fmt.Sprintf("nonstream-keepalive-interval: %d -> %d", oldCfg.NonStreamKeepAliveInterval, newCfg.NonStreamKeepAliveInterval))
	}

	// Quota-exceeded behavior
	if oldCfg.QuotaExceeded.SwitchProject != newCfg.QuotaExceeded.SwitchProject {
		changes = append(changes, fmt.Sprintf("quota-exceeded.switch-project: %t -> %t", oldCfg.QuotaExceeded.SwitchProject, newCfg.QuotaExceeded.SwitchProject))
	}
	if oldCfg.Routing.Strategy != newCfg.Routing.Strategy {
		changes = append(changes, fmt.Sprintf("routing.strategy: %s -> %s", oldCfg.Routing.Strategy, newCfg.Routing.Strategy))
	}

	// API keys (redacted) and counts
	if len(oldCfg.APIKeys) != len(newCfg.APIKeys) {
		changes = append(changes, fmt.Sprintf("api-keys count: %d -> %d", len(oldCfg.APIKeys), len(newCfg.APIKeys)))
	} else if !reflect.DeepEqual(trimStrings(oldCfg.APIKeys), trimStrings(newCfg.APIKeys)) {
		changes = append(changes, "api-keys: values updated (count unchanged, redacted)")
	}
	// Codex keys (do not print key material)
	if len(oldCfg.CodexKey) != len(newCfg.CodexKey) {
		changes = append(changes, fmt.Sprintf("codex-api-key count: %d -> %d", len(oldCfg.CodexKey), len(newCfg.CodexKey)))
	} else {
		for i := range oldCfg.CodexKey {
			o := oldCfg.CodexKey[i]
			n := newCfg.CodexKey[i]
			if strings.TrimSpace(o.BaseURL) != strings.TrimSpace(n.BaseURL) {
				changes = append(changes, fmt.Sprintf("codex[%d].base-url: %s -> %s", i, strings.TrimSpace(o.BaseURL), strings.TrimSpace(n.BaseURL)))
			}
			if o.Websockets != n.Websockets {
				changes = append(changes, fmt.Sprintf("codex[%d].websockets: %t -> %t", i, o.Websockets, n.Websockets))
			}
			if strings.TrimSpace(o.APIKey) != strings.TrimSpace(n.APIKey) {
				changes = append(changes, fmt.Sprintf("codex[%d].api-key: updated", i))
			}
			if o.Priority != n.Priority {
				changes = append(changes, fmt.Sprintf("codex[%d].priority: %d -> %d", i, o.Priority, n.Priority))
			}
			if !equalStringMap(o.Headers, n.Headers) {
				changes = append(changes, fmt.Sprintf("codex[%d].headers: updated", i))
			}
		}
	}
	if oldCfg.CodexHeaderDefaults.UserAgent != newCfg.CodexHeaderDefaults.UserAgent {
		changes = append(changes, fmt.Sprintf("codex-header-defaults.user-agent: %s -> %s", oldCfg.CodexHeaderDefaults.UserAgent, newCfg.CodexHeaderDefaults.UserAgent))
	}
	if oldCfg.CodexHeaderDefaults.BetaFeatures != newCfg.CodexHeaderDefaults.BetaFeatures {
		changes = append(changes, fmt.Sprintf("codex-header-defaults.beta-features: %s -> %s", oldCfg.CodexHeaderDefaults.BetaFeatures, newCfg.CodexHeaderDefaults.BetaFeatures))
	}

	return changes
}

func trimStrings(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = strings.TrimSpace(in[i])
	}
	return out
}

func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
