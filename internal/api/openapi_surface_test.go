package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAPIDocMatchesRedesignedManagementSurface(t *testing.T) {
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
		"/ws-auth:",
		"/request-retry:",
		"/max-retry-interval:",
		"/routing/strategy:",
		"/quota-exceeded/switch-project:",
		"/codex-api-key:",
		"/api-call:",
		"/auth-files/download:",
		"/auth-files/status:",
		"/auth-files/fields:",
		"/codex-auth-url:",
		"/oauth-callback:",
		"/get-auth-status:",
		"CodexKey:",
		"ManagementApiCallRequest:",
	} {
		if strings.Contains(text, banned) {
			t.Fatalf("did not expect %q in api/openapi.yaml", banned)
		}
	}

	for _, required := range []string{
		"/runtime-settings:",
		"/api-keys:",
		"/auth-files:",
		"/auth-files/{name}/content:",
		"/auth-files/{name}:",
		"/auth-files/{name}/usage:",
		"/oauth-sessions:",
		"/oauth-sessions/{state}:",
		"/oauth-sessions/{state}/callback:",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected %q in api/openapi.yaml", required)
		}
	}
}

func TestOpenAPIDocRuntimeSettingsSchema(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	text := string(contents)

	start := strings.Index(text, "    RuntimeSettings:\n")
	if start < 0 {
		t.Fatal("expected RuntimeSettings schema in openapi")
	}
	end := strings.Index(text[start:], "    APIKeysEnvelope:\n")
	if end < 0 {
		t.Fatal("expected APIKeysEnvelope schema after RuntimeSettings in openapi")
	}
	section := text[start : start+end]

	for _, required := range []string{
		"ws-auth:",
		"request-retry:",
		"max-retry-interval:",
		"routing-strategy:",
		"switch-project:",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("expected %q in RuntimeSettings schema: %s", required, section)
		}
	}
}

func TestOpenAPIDocAuthFileSchemaDropsLegacyProbe(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	text := string(contents)

	start := strings.Index(text, "    AuthFile:\n")
	if start < 0 {
		t.Fatal("expected AuthFile schema in openapi")
	}
	end := strings.Index(text[start:], "    AuthFilesEnvelope:\n")
	if end < 0 {
		t.Fatal("expected AuthFilesEnvelope schema after AuthFile in openapi")
	}
	section := text[start : start+end]

	for _, banned := range []string{
		"usage_probe:",
		"prefix:",
	} {
		if strings.Contains(section, banned) {
			t.Fatalf("did not expect %q in AuthFile schema: %s", banned, section)
		}
	}

	for _, required := range []string{
		"priority:",
		"usage:",
		"usage_available:",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("expected %q in AuthFile schema: %s", required, section)
		}
	}
}

func TestOpenAPIDocOAuthSessionSchemas(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	text := string(contents)

	createStart := strings.Index(text, "    OAuthSessionCreateRequest:\n")
	if createStart < 0 {
		t.Fatal("expected OAuthSessionCreateRequest schema in openapi")
	}
	createEnd := strings.Index(text[createStart:], "    OAuthSessionCreateResponse:\n")
	if createEnd < 0 {
		t.Fatal("expected OAuthSessionCreateResponse schema after OAuthSessionCreateRequest in openapi")
	}
	createSection := text[createStart : createStart+createEnd]
	for _, required := range []string{
		"provider:",
		"callback_origin:",
	} {
		if !strings.Contains(createSection, required) {
			t.Fatalf("expected %q in OAuthSessionCreateRequest schema: %s", required, createSection)
		}
	}

	statusStart := strings.Index(text, "    OAuthSessionStatusResponse:\n")
	if statusStart < 0 {
		t.Fatal("expected OAuthSessionStatusResponse schema in openapi")
	}
	statusEnd := strings.Index(text[statusStart:], "    OAuthSessionCallbackRequest:\n")
	if statusEnd < 0 {
		t.Fatal("expected OAuthSessionCallbackRequest schema after OAuthSessionStatusResponse in openapi")
	}
	statusSection := text[statusStart : statusStart+statusEnd]
	for _, required := range []string{
		"enum: [pending, complete, error]",
		"provider:",
	} {
		if !strings.Contains(statusSection, required) {
			t.Fatalf("expected %q in OAuthSessionStatusResponse schema: %s", required, statusSection)
		}
	}
}
