package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTask6AdjacentCleanupRemovesDeadFeatureFiles(t *testing.T) {
	for _, rel := range []string{
		"sdk/cliproxy/service_models_helpers.go",
		"internal/runtime/executor/payload_helpers.go",
		"internal/watcher/diff/excluded_hash.go",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", rel)); !os.IsNotExist(err) {
			if err == nil {
				t.Fatalf("did not expect %s to exist", rel)
			}
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}
