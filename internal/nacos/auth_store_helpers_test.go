package nacos

import (
	"strings"
	"testing"
)

func TestParseAuthEntriesSupportsObjectMap(t *testing.T) {
	raw := `{"alpha":{"file_name":"alpha.json","type":"codex","email":"alpha@example.com","disabled":false}}`

	entries, err := parseAuthEntries(raw)
	if err != nil {
		t.Fatalf("parseAuthEntries returned error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 auth entry, got %d", len(entries))
	}
	if got := stringValue(entries["alpha"], "email"); got != "alpha@example.com" {
		t.Fatalf("expected alpha email alpha@example.com, got %q", got)
	}
}

func TestParseAuthEntriesRejectsMissingFileName(t *testing.T) {
	raw := `{"alpha":{"type":"codex","email":"alpha@example.com","disabled":false}}`

	_, err := parseAuthEntries(raw)
	if err == nil {
		t.Fatal("expected parseAuthEntries to reject missing file_name")
	}
	if !strings.Contains(err.Error(), "missing file_name") {
		t.Fatalf("expected missing file_name error, got %v", err)
	}
}

func TestParseAuthEntriesSupportsJSONArray(t *testing.T) {
	raw := `[
		{"id":"alpha.json","type":"codex","email":"alpha@example.com","disabled":false}
	]`

	_, err := parseAuthEntries(raw)
	if err == nil {
		t.Fatal("expected parseAuthEntries to reject JSON arrays")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal array") {
		t.Fatalf("expected array rejection error, got %v", err)
	}
}

func TestParseAuthEntriesRejectsPathLikeExplicitIdentifiers(t *testing.T) {
	raw := `{"nested/alpha.json":{"type":"codex","email":"alpha@example.com","disabled":false}}`

	_, err := parseAuthEntries(raw)
	if err == nil {
		t.Fatal("expected parseAuthEntries to reject path-like auth id")
	}
	if !strings.Contains(err.Error(), "invalid auth id") {
		t.Fatalf("expected invalid auth id error, got %v", err)
	}
}
