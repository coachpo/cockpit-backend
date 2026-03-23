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
		"/model-definitions/{channel}:",
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
		"/auth-files/models:",
		"/auth-files/download:",
		"/auth-files/status:",
		"/auth-files/fields:",
		"/api-call:",
		"/codex-auth-url:",
		"/oauth-callback:",
		"/get-auth-status:",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected %q in api/openapi.yaml", required)
		}
	}
}

func TestOpenAPIDocRetainsManagementAPICallContract(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	text := string(contents)

	schemaStart := strings.Index(text, "    ManagementApiCallRequest:\n")
	if schemaStart < 0 {
		t.Fatal("expected ManagementApiCallRequest schema in openapi")
	}
	schemaEnd := strings.Index(text[schemaStart:], "    CodexKey:\n")
	if schemaEnd < 0 {
		t.Fatal("expected CodexKey schema after ManagementApiCallRequest in openapi")
	}
	schemaSection := text[schemaStart : schemaStart+schemaEnd]

	for _, required := range []string{
		"required: [authIndex, method, url]",
		"authIndex:",
		"method:",
		"url:",
		"header:",
		"body:",
	} {
		if !strings.Contains(schemaSection, required) {
			t.Fatalf("expected %q in ManagementApiCallRequest schema: %s", required, schemaSection)
		}
	}

	pathStart := strings.Index(text, "  /api-call:\n")
	if pathStart < 0 {
		t.Fatal("expected /api-call path in openapi")
	}
	pathEnd := strings.Index(text[pathStart:], "  /auth-files:\n")
	if pathEnd < 0 {
		pathEnd = strings.Index(text[pathStart:], "  /codex-auth-url:\n")
	}
	if pathEnd < 0 {
		t.Fatal("expected another path after /api-call in openapi")
	}
	pathSection := text[pathStart : pathStart+pathEnd]

	for _, required := range []string{
		"summary: Execute a provider-authenticated management usage probe",
		"$ref: '#/components/schemas/ManagementApiCallRequest'",
		"description: Upstream JSON payload",
		"additionalProperties: true",
		"description: Invalid probe request",
		"description: Auth file not found",
		"description: Upstream request or response read failed",
		"description: Auth manager unavailable",
	} {
		if !strings.Contains(pathSection, required) {
			t.Fatalf("expected %q in /api-call path: %s", required, pathSection)
		}
	}

	for _, banned := range []string{
		"summary: Proxy-aware upstream HTTP call",
		"auth_index:",
		"status_code:",
		"description: API call response",
	} {
		if strings.Contains(pathSection, banned) {
			t.Fatalf("did not expect %q in /api-call path: %s", banned, pathSection)
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
