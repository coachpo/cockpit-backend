package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlersSourceMatchesBaseAPIHandlerShape(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("handlers.go"))
	if err != nil {
		t.Fatalf("read handlers.go: %v", err)
	}
	text := string(contents)
	for _, banned := range []string{
		"shared across all API endpoint handlers (OpenAI, Claude, Gemini)",
		"holds a pool of clients",
		"load balancing, client selection",
		"cliClients",
		"A slice of AI service clients",
		"UpdateClients(",
	} {
		if strings.Contains(text, banned) {
			t.Fatalf("did not expect %q in handlers.go", banned)
		}
	}
	if !strings.Contains(text, "UpdateConfig(") {
		t.Fatal("expected handlers.go to expose UpdateConfig")
	}
}
