package config

import (
	"os"
	"path/filepath"
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

	assertMapHasKey(t, example, "nonstream-keepalive-interval")
	assertMapHasKey(t, example, "oauth-excluded-models")
	assertMapHasKey(t, example, "oauth-model-alias")

	streaming := assertNestedMap(t, example, "streaming")
	assertMapHasKey(t, streaming, "keepalive-seconds")
	assertMapHasKey(t, streaming, "bootstrap-retries")

	codexEntries := assertNestedList(t, example, "codex-api-key")
	if len(codexEntries) == 0 {
		t.Fatal("expected config example to include a codex-api-key entry")
	}
	codexEntry, ok := codexEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected codex-api-key entry to be a map, got %T", codexEntries[0])
	}
	for _, key := range []string{"api-key", "priority", "prefix", "base-url", "websockets", "proxy-url", "models", "headers", "excluded-models"} {
		assertMapHasKey(t, codexEntry, key)
	}

	compatEntries := assertNestedList(t, example, "openai-compatibility")
	if len(compatEntries) == 0 {
		t.Fatal("expected config example to include an openai-compatibility entry")
	}
	compatEntry, ok := compatEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected openai-compatibility entry to be a map, got %T", compatEntries[0])
	}
	for _, key := range []string{"name", "priority", "prefix", "base-url", "api-key-entries", "models", "headers"} {
		assertMapHasKey(t, compatEntry, key)
	}
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
