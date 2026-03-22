package cliproxy

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
)

func TestAPIKeyClientResultOnlyExposesCodexCount(t *testing.T) {
	typ := reflect.TypeOf(APIKeyClientResult{})
	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	sort.Strings(got)
	want := []string{"CodexKeyCount"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("unexpected APIKeyClientResult fields: got %v want %v", got, want)
	}
}

func TestAPIKeyClientProviderLoadReturnsCodexCount(t *testing.T) {
	provider := &apiKeyClientProvider{}
	result, err := provider.Load(context.Background(), &config.Config{
		CodexKey: []config.CodexKey{{APIKey: "k1"}, {APIKey: "k2"}},
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CodexKeyCount != 2 {
		t.Fatalf("expected CodexKeyCount=2, got %d", result.CodexKeyCount)
	}
}

func TestServiceSourceDropsUnusedProviderResults(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("service.go"))
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	text := string(contents)
	for _, banned := range []string{
		"tokenResult",
		"apiKeyResult",
		"RegisterUsagePlugin registers a usage plugin on the global usage manager",
		"registerModelsForAuth",
	} {
		if strings.Contains(text, banned) {
			t.Fatalf("did not expect %q in service.go", banned)
		}
	}
}
