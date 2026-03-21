package nacos

import "testing"

func TestParseAuthEntriesSupportsObjectMap(t *testing.T) {
	raw := `{"alpha":{"type":"codex","email":"alpha@example.com","disabled":false}}`

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

func TestParseAuthEntriesSupportsJSONArray(t *testing.T) {
	raw := `[
		{"file_name":"alpha.json","type":"codex","email":"alpha@example.com","disabled":false},
		{"account_id":"acct-2","type":"codex","email":"beta@example.com","disabled":true}
	]`

	entries, err := parseAuthEntries(raw)
	if err != nil {
		t.Fatalf("parseAuthEntries returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 auth entries, got %d", len(entries))
	}

	if got := stringValue(entries["alpha.json"], "email"); got != "alpha@example.com" {
		t.Fatalf("expected alpha.json email alpha@example.com, got %q", got)
	}

	derivedID := "codex-acct-2.json"
	if got := stringValue(entries[derivedID], "email"); got != "beta@example.com" {
		t.Fatalf("expected %s email beta@example.com, got %q", derivedID, got)
	}
	if disabled, _ := entries[derivedID]["disabled"].(bool); !disabled {
		t.Fatalf("expected %s to stay disabled", derivedID)
	}
}
