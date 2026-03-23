package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAPIDocMatchesTrimmedManagementSurface(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	text := string(contents)

	for _, banned := range []string{
		"BearerAuth:",
		"securitySchemes:",
		"security:\n  - BearerAuth: []",
		"ManagementKeyHeader:",
		"- ManagementKeyHeader: []",
		"/config:",
		"/config.yaml:",
		"/latest-version:",
		"/debug:",
		"/request-log:",
		"/proxy-url:",
		"/quota-exceeded/switch-preview-model:",
		"/force-model-prefix:",
		"/oauth-excluded-models:",
		"/oauth-model-alias:",
		"OAuthModelAlias:",
	} {
		if strings.Contains(text, banned) {
			t.Fatalf("did not expect %q in api/openapi.yaml", banned)
		}
	}

	for _, required := range []string{
		"/ws-auth:",
		"/request-retry:",
		"/max-retry-interval:",
		"/routing/strategy:",
		"/quota-exceeded/switch-project:",
		"/api-keys:",
		"/codex-api-key:",
		"/auth-files:",
		"/api-call:",
		"/auth-files/download:",
		"/auth-files/status:",
		"/auth-files/fields:",
		"/model-definitions/{channel}:",
		"/codex-auth-url:",
		"/oauth-callback:",
		"/get-auth-status:",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected %q in api/openapi.yaml", required)
		}
	}

	for _, bannedNow := range []string{
		"/auth-files/models:",
		"prefix:",
	} {
		if strings.Contains(text, bannedNow) {
			t.Fatalf("did not expect %q in api/openapi.yaml", bannedNow)
		}
	}
}

func TestOpenAPIDocTrimmedCodexKeySchema(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	text := string(contents)

	start := strings.Index(text, "    CodexKey:\n")
	if start < 0 {
		t.Fatal("expected CodexKey schema in openapi")
	}
	end := strings.Index(text[start:], "    AuthFile:\n")
	if end < 0 {
		t.Fatal("expected AuthFile schema after CodexKey in openapi")
	}
	section := text[start : start+end]

	for _, banned := range []string{
		"prefix:",
		"proxy-url:",
		"models:",
		"alias:",
		"excluded-models:",
	} {
		if strings.Contains(section, banned) {
			t.Fatalf("did not expect %q in CodexKey schema: %s", banned, section)
		}
	}

	for _, required := range []string{
		"api-key:",
		"priority:",
		"base-url:",
		"websockets:",
		"headers:",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("expected %q in CodexKey schema: %s", required, section)
		}
	}
}
