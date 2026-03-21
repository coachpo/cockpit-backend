package auth

import (
	"testing"

	internalconfig "github.com/coachpo/cockpit-backend/internal/config"
)

func TestOAuthModelAliasChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		authKind string
		want     string
	}{
		{
			name:     "codex oauth keeps codex channel",
			provider: "codex",
			authKind: "oauth",
			want:     "codex",
		},
		{
			name:     "codex apikey disables alias channel",
			provider: "codex",
			authKind: "apikey",
			want:     "",
		},
		{
			name:     "claude no longer has alias channel",
			provider: "claude",
			authKind: "oauth",
			want:     "",
		},
		{
			name:     "gemini cli no longer has alias channel",
			provider: "gemini-cli",
			authKind: "oauth",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := OAuthModelAliasChannel(tt.provider, tt.authKind)
			if got != tt.want {
				t.Fatalf("OAuthModelAliasChannel(%q, %q) = %q, want %q", tt.provider, tt.authKind, got, tt.want)
			}
		})
	}
}

func TestResolveOAuthUpstreamModel_ChannelScoping(t *testing.T) {
	t.Parallel()

	aliases := map[string][]internalconfig.OAuthModelAlias{
		"codex":      {{Name: "gpt-5-codex", Alias: "gpt-5"}},
		"claude":     {{Name: "claude-sonnet-4-5-20250514", Alias: "claude-sonnet-4-5"}},
		"gemini-cli": {{Name: "gemini-2.5-pro-exp-03-25", Alias: "gemini-2.5-pro"}},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(&internalconfig.Config{})
	mgr.SetOAuthModelAlias(aliases)

	tests := []struct {
		name  string
		auth  *Auth
		input string
		want  string
	}{
		{
			name:  "codex oauth preserves suffix",
			auth:  &Auth{Provider: "codex", Attributes: map[string]string{"auth_kind": "oauth"}},
			input: "gpt-5(8192)",
			want:  "gpt-5-codex(8192)",
		},
		{
			name:  "codex apikey skips alias resolution",
			auth:  &Auth{Provider: "codex", Attributes: map[string]string{"auth_kind": "apikey"}},
			input: "gpt-5(8192)",
			want:  "",
		},
		{
			name:  "claude aliases no longer resolve",
			auth:  &Auth{Provider: "claude", Attributes: map[string]string{"auth_kind": "oauth"}},
			input: "claude-sonnet-4-5(high)",
			want:  "",
		},
		{
			name:  "gemini cli aliases no longer resolve",
			auth:  &Auth{Provider: "gemini-cli"},
			input: "gemini-2.5-pro(8192)",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mgr.resolveOAuthUpstreamModel(tt.auth, tt.input)
			if got != tt.want {
				t.Fatalf("resolveOAuthUpstreamModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestApplyOAuthModelAlias_CodexPreservesSuffix(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(&internalconfig.Config{})
	mgr.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
		"codex": {{Name: "gpt-5-codex", Alias: "gpt-5"}},
	})

	auth := &Auth{Provider: "codex", Attributes: map[string]string{"auth_kind": "oauth"}}

	resolvedModel := mgr.applyOAuthModelAlias(auth, "gpt-5(8192)")
	if resolvedModel != "gpt-5-codex(8192)" {
		t.Fatalf("applyOAuthModelAlias() model = %q, want %q", resolvedModel, "gpt-5-codex(8192)")
	}
}
