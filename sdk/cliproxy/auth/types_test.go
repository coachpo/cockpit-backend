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

	geminiAuth := &Auth{
		Provider: "gemini",
		Attributes: map[string]string{
			"api_key": "shared-key",
			"source":  "config:gemini[abc123]",
		},
	}
	compatAuth := &Auth{
		Provider: "bohe",
		Attributes: map[string]string{
			"api_key":      "shared-key",
			"compat_name":  "bohe",
			"provider_key": "bohe",
			"source":       "config:bohe[def456]",
		},
	}
	geminiAltBase := &Auth{
		Provider: "gemini",
		Attributes: map[string]string{
			"api_key":  "shared-key",
			"base_url": "https://alt.example.com",
			"source":   "config:gemini[ghi789]",
		},
	}
	geminiDuplicate := &Auth{
		Provider: "gemini",
		Attributes: map[string]string{
			"api_key": "shared-key",
			"source":  "config:gemini[abc123-1]",
		},
	}

	geminiIndex := geminiAuth.EnsureIndex()
	compatIndex := compatAuth.EnsureIndex()
	altBaseIndex := geminiAltBase.EnsureIndex()
	duplicateIndex := geminiDuplicate.EnsureIndex()

	if geminiIndex == "" {
		t.Fatal("gemini index should not be empty")
	}
	if compatIndex == "" {
		t.Fatal("compat index should not be empty")
	}
	if altBaseIndex == "" {
		t.Fatal("alt base index should not be empty")
	}
	if duplicateIndex == "" {
		t.Fatal("duplicate index should not be empty")
	}
	if geminiIndex == compatIndex {
		t.Fatalf("shared api key produced duplicate auth_index %q", geminiIndex)
	}
	if geminiIndex == altBaseIndex {
		t.Fatalf("same provider/key with different base_url produced duplicate auth_index %q", geminiIndex)
	}
	if geminiIndex == duplicateIndex {
		t.Fatalf("duplicate config entries should be separated by source-derived seed, got %q", geminiIndex)
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
				Provider:   "openai",
				Attributes: map[string]string{"api_key": "sk-test"},
			},
			wantType:  "api_key",
			wantValue: "sk-test",
		},
		{
			name: "gemini-cli project_id metadata is ignored in oauth account info",
			auth: &Auth{
				Provider: "gemini-cli",
				Metadata: map[string]any{
					"email":      "user@example.com",
					"project_id": "project-123",
				},
			},
			wantType:  "oauth",
			wantValue: "user@example.com",
		},
		{
			name: "iflow email metadata follows generic oauth behavior",
			auth: &Auth{
				Provider: "iflow",
				Metadata: map[string]any{"email": "iflow-user@example.com"},
			},
			wantType:  "oauth",
			wantValue: "iflow-user@example.com",
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
