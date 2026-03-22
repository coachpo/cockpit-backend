package auth

import "testing"

func TestToolPrefixDisabled(t *testing.T) {
	var a *Auth
	if a.ToolPrefixDisabled() {
		t.Error("nil auth should return false")
	}

	a = &Auth{}
	if a.ToolPrefixDisabled() {
		t.Error("empty auth should return false")
	}

	a = &Auth{Metadata: map[string]any{"tool_prefix_disabled": true}}
	if !a.ToolPrefixDisabled() {
		t.Error("should return true when set to true")
	}

	a = &Auth{Metadata: map[string]any{"tool_prefix_disabled": "true"}}
	if !a.ToolPrefixDisabled() {
		t.Error("should return true when set to string 'true'")
	}

	a = &Auth{Metadata: map[string]any{"tool-prefix-disabled": true}}
	if !a.ToolPrefixDisabled() {
		t.Error("should return true with kebab-case key")
	}

	a = &Auth{Metadata: map[string]any{"tool_prefix_disabled": false}}
	if a.ToolPrefixDisabled() {
		t.Error("should return false when set to false")
	}
}

func TestEnsureIndexUsesCredentialIdentity(t *testing.T) {
	t.Parallel()

	primaryAuth := &Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key": "shared-key",
			"source":  "config:codex[abc123]",
		},
	}
	secondaryAuth := &Auth{
		Provider: "custom",
		Attributes: map[string]string{
			"api_key": "shared-key",
			"source":  "config:custom[def456]",
		},
	}
	primaryAltBase := &Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "shared-key",
			"base_url": "https://alt.example.com",
			"source":   "config:codex[ghi789]",
		},
	}
	primaryDuplicate := &Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key": "shared-key",
			"source":  "config:codex[abc123-1]",
		},
	}

	primaryIndex := primaryAuth.EnsureIndex()
	secondaryIndex := secondaryAuth.EnsureIndex()
	altBaseIndex := primaryAltBase.EnsureIndex()
	duplicateIndex := primaryDuplicate.EnsureIndex()

	if primaryIndex == "" {
		t.Fatal("primary index should not be empty")
	}
	if secondaryIndex == "" {
		t.Fatal("secondary index should not be empty")
	}
	if altBaseIndex == "" {
		t.Fatal("alt base index should not be empty")
	}
	if duplicateIndex == "" {
		t.Fatal("duplicate index should not be empty")
	}
	if primaryIndex == secondaryIndex {
		t.Fatalf("shared api key produced duplicate auth_index %q", primaryIndex)
	}
	if primaryIndex == altBaseIndex {
		t.Fatalf("same provider/key with different base_url produced duplicate auth_index %q", primaryIndex)
	}
	if primaryIndex == duplicateIndex {
		t.Fatalf("duplicate config entries should be separated by source-derived seed, got %q", primaryIndex)
	}
}

func TestExecutorKeyFromAuth_UsesProviderOnly(t *testing.T) {
	t.Parallel()

	auth := &Auth{
		Provider: "CoDeX",
		Attributes: map[string]string{
			"provider_key": "ignored-provider-attr",
			"compat_name":  "ignored-name-attr",
		},
	}

	if got := executorKeyFromAuth(auth); got != "codex" {
		t.Fatalf("executorKeyFromAuth() = %q, want %q", got, "codex")
	}
}

func TestAccountInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		auth      *Auth
		wantType  string
		wantValue string
	}{
		{
			name:      "nil auth returns empty values",
			auth:      nil,
			wantType:  "",
			wantValue: "",
		},
		{
			name:      "empty auth returns empty values",
			auth:      &Auth{},
			wantType:  "",
			wantValue: "",
		},
		{
			name: "oauth account info uses metadata email",
			auth: &Auth{
				Provider: "codex",
				Metadata: map[string]any{"email": "  user@example.com  "},
			},
			wantType:  "oauth",
			wantValue: "user@example.com",
		},
		{
			name: "whitespace-only email is treated as empty",
			auth: &Auth{
				Provider: "codex",
				Metadata: map[string]any{"email": "   "},
			},
			wantType:  "",
			wantValue: "",
		},
		{
			name: "api key account info uses attributes api_key",
			auth: &Auth{
				Provider:   "codex",
				Attributes: map[string]string{"api_key": "sk-test"},
			},
			wantType:  "api_key",
			wantValue: "sk-test",
		},
		{
			name: "project_id metadata is ignored in oauth account info",
			auth: &Auth{
				Provider: "codex",
				Metadata: map[string]any{
					"email":      "user@example.com",
					"project_id": "project-123",
				},
			},
			wantType:  "oauth",
			wantValue: "user@example.com",
		},
		{
			name: "oauth email metadata follows generic oauth behavior",
			auth: &Auth{
				Provider: "codex",
				Metadata: map[string]any{"email": "oauth-user@example.com"},
			},
			wantType:  "oauth",
			wantValue: "oauth-user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotValue := tt.auth.AccountInfo()
			if gotType != tt.wantType || gotValue != tt.wantValue {
				t.Fatalf("AccountInfo() = (%q, %q), want (%q, %q)", gotType, gotValue, tt.wantType, tt.wantValue)
			}
		})
	}
}
