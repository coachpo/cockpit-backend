package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestConfigSchemaMatchesTrimmedSurface(t *testing.T) {
	assertStructFieldsExactly(t, reflect.TypeOf(Config{}), []string{
		"Host",
		"Port",
		"RemoteManagement",
		"AuthDir",
		"DisableCooling",
		"RequestRetry",
		"MaxRetryCredentials",
		"MaxRetryInterval",
		"QuotaExceeded",
		"Routing",
		"WebsocketAuth",
		"CodexKey",
		"CodexHeaderDefaults",
	})

	assertStructFieldsExactly(t, reflect.TypeOf(SDKConfig{}), []string{
		"APIKeys",
		"PassthroughHeaders",
		"Streaming",
		"NonStreamKeepAliveInterval",
	})

	assertStructFieldsExactly(t, reflect.TypeOf(QuotaExceeded{}), []string{"SwitchProject"})
	assertStructFieldsExactly(t, reflect.TypeOf(RemoteManagement{}), []string{"AllowRemote", "SecretKey"})
	assertStructFieldsExactly(t, reflect.TypeOf(RoutingConfig{}), []string{"Strategy"})
	assertStructFieldsExactly(t, reflect.TypeOf(CodexHeaderDefaults{}), []string{"UserAgent", "BetaFeatures"})
	assertStructFieldsExactly(t, reflect.TypeOf(StreamingConfig{}), []string{"KeepAliveSeconds", "BootstrapRetries"})

	assertStructFieldsExactly(t, reflect.TypeOf(CodexKey{}), []string{
		"APIKey",
		"Priority",
		"BaseURL",
		"Websockets",
		"Headers",
	})

	assertConfigGoTypeNamesExactly(t, []string{
		"Config",
		"CodexHeaderDefaults",
		"RemoteManagement",
		"QuotaExceeded",
		"RoutingConfig",
		"CodexKey",
	})
}

func assertStructFieldsExactly(t *testing.T, typ reflect.Type, want []string) {
	t.Helper()
	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Anonymous {
			continue
		}
		got = append(got, field.Name)
	}
	sort.Strings(got)
	wantCopy := append([]string(nil), want...)
	sort.Strings(wantCopy)
	assertStringSlicesEqual(t, got, wantCopy)
}

func assertConfigGoTypeNamesExactly(t *testing.T, want []string) {
	t.Helper()
	fileSet := token.NewFileSet()
	filePath := filepath.Join("config.go")
	file, err := parser.ParseFile(fileSet, filePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filePath, err)
	}
	got := make([]string, 0)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			got = append(got, typeSpec.Name.Name)
		}
	}
	sort.Strings(got)
	wantCopy := append([]string(nil), want...)
	sort.Strings(wantCopy)
	assertStringSlicesEqual(t, got, wantCopy)
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mismatch: got %v want %v", got, want)
		}
	}
}
