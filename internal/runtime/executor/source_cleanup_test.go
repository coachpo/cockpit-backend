package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTask6PayloadRewriteHelpersStayRemoved(t *testing.T) {
	path := filepath.Join("payload_helpers.go")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		if err == nil {
			t.Fatalf("did not expect %s to exist", path)
		}
		t.Fatalf("stat %s: %v", path, err)
	}
}

func TestTask6LiveExecutorsDoNotReintroducePayloadRewriteSnippets(t *testing.T) {
	tests := []struct {
		path   string
		banned []string
	}{
		{
			path: "codex_executor.go",
			banned: []string{
				`sjson.SetBytes(body, "model",`,
				`sjson.SetBytes(body, "stream",`,
				`sjson.DeleteBytes(body, "stream")`,
				`sjson.DeleteBytes(body, "previous_response_id")`,
				`sjson.DeleteBytes(body, "prompt_cache_retention")`,
				`sjson.DeleteBytes(body, "safety_identifier")`,
				`sjson.SetBytes(body, "instructions",`,
			},
		},
		{
			path: "codex_websockets_execute.go",
			banned: []string{
				`sjson.SetBytes(body, "model",`,
				`sjson.SetBytes(body, "stream",`,
				`sjson.DeleteBytes(body, "previous_response_id")`,
				`sjson.DeleteBytes(body, "prompt_cache_retention")`,
				`sjson.DeleteBytes(body, "safety_identifier")`,
				`sjson.SetBytes(body, "instructions",`,
				`normalizeCodexWebsocketCompletion(payload)`,
			},
		},
		{
			path: "codex_websockets_stream.go",
			banned: []string{
				`normalizeCodexWebsocketCompletion(payload)`,
			},
		},
		{
			path: "codex_websocket_helpers.go",
			banned: []string{
				`rawJSON, _ = sjson.SetBytes(rawJSON, "prompt_cache_key", cache.ID)`,
				`func normalizeCodexWebsocketCompletion(payload []byte) []byte {`,
				`sjson.SetBytes(payload, "type", "response.completed")`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(tt.path))
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}
			text := string(contents)
			for _, banned := range tt.banned {
				if strings.Contains(text, banned) {
					t.Fatalf("did not expect %q in %s", banned, tt.path)
				}
			}
		})
	}
}
