package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigExampleIncludesCurrentInventory(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatalf("read config.example.yaml: %v", err)
	}

	var example map[string]any
	if err := yaml.Unmarshal(raw, &example); err != nil {
		t.Fatalf("unmarshal config.example.yaml: %v", err)
	}

	assertExactMapKeys(t, example, []string{
		"api-keys",
		"auth-dir",
		"codex-api-key",
		"codex-header-defaults",
		"disable-cooling",
		"host",
		"max-retry-credentials",
		"max-retry-interval",
		"nonstream-keepalive-interval",
		"passthrough-headers",
		"port",
		"quota-exceeded",
		"request-retry",
		"routing",
		"streaming",
		"ws-auth",
	})

	quotaExceeded := assertNestedMap(t, example, "quota-exceeded")
	assertExactMapKeys(t, quotaExceeded, []string{"switch-project"})

	routing := assertNestedMap(t, example, "routing")
	assertExactMapKeys(t, routing, []string{"strategy"})

	streaming := assertNestedMap(t, example, "streaming")
	assertExactMapKeys(t, streaming, []string{"bootstrap-retries", "keepalive-seconds"})

	codexHeaderDefaults := assertNestedMap(t, example, "codex-header-defaults")
	assertExactMapKeys(t, codexHeaderDefaults, []string{"beta-features", "user-agent"})

	codexEntries := assertNestedList(t, example, "codex-api-key")
	if len(codexEntries) == 0 {
		t.Fatal("expected config example to include a codex-api-key entry")
	}
	codexEntry, ok := codexEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected codex-api-key entry to be a map, got %T", codexEntries[0])
	}
	assertExactMapKeys(t, codexEntry, []string{"api-key", "base-url", "headers", "priority", "websockets"})
}

func assertNestedList(t *testing.T, source map[string]any, key string) []any {
	t.Helper()
	value, ok := source[key]
	if !ok {
		t.Fatalf("expected key %q in config example", key)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected key %q to be a list, got %T", key, value)
	}
	return items
}

func assertNestedMap(t *testing.T, source map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := source[key]
	if !ok {
		t.Fatalf("expected key %q in config example", key)
	}
	child, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected key %q to be a map, got %T", key, value)
	}
	return child
}

func assertMapHasKey(t *testing.T, source map[string]any, key string) {
	t.Helper()
	if _, ok := source[key]; !ok {
		t.Fatalf("expected key %q in config example", key)
	}
}

func assertExactMapKeys(t *testing.T, source map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(source))
	for key := range source {
		got = append(got, key)
	}
	sort.Strings(got)
	wantCopy := append([]string(nil), want...)
	sort.Strings(wantCopy)
	if len(got) != len(wantCopy) {
		t.Fatalf("config example keys mismatch: got %v want %v", got, wantCopy)
	}
	for i := range wantCopy {
		if got[i] != wantCopy[i] {
			t.Fatalf("config example keys mismatch: got %v want %v", got, wantCopy)
		}
	}
}
