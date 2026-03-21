package wsrelay

import (
	"strings"
	"testing"
)

func TestRandomProviderNameUsesWSRelayPrefix(t *testing.T) {
	t.Parallel()

	name := randomProviderName()
	if !strings.HasPrefix(name, "wsrelay-") {
		t.Fatalf("expected wsrelay prefix, got %q", name)
	}
	if name == "wsrelay-" {
		t.Fatal("expected random provider name suffix")
	}
}
