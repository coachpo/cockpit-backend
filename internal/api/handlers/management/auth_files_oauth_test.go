package management

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

func TestBuildCodexOAuthRecord_PreservesDisplayMetadataAndActiveStatus(t *testing.T) {
	const (
		email       = "oauth@example.com"
		accountID   = "acct_123"
		planType    = "plus"
		lastRefresh = "2026-03-22T19:00:00Z"
		expireAt    = "2026-03-23T19:00:00Z"
	)

	idToken := testCodexIDToken(t, email, accountID, planType)
	record := buildCodexOAuthRecord(&codex.CodexAuthBundle{
		TokenData: codex.CodexTokenData{
			IDToken:      idToken,
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			AccountID:    accountID,
			Email:        email,
			Expire:       expireAt,
		},
		LastRefresh: lastRefresh,
	})

	if record == nil {
		t.Fatal("expected auth record")
	}
	if record.Status != coreauth.StatusActive {
		t.Fatalf("expected OAuth auth status %q, got %q", coreauth.StatusActive, record.Status)
	}
	if record.Provider != "codex" {
		t.Fatalf("expected codex provider, got %q", record.Provider)
	}
	if record.Metadata["id_token"] != idToken {
		t.Fatalf("expected id_token metadata to be preserved, got %#v", record.Metadata["id_token"])
	}
	if record.Metadata["access_token"] != "access-token" {
		t.Fatalf("expected access_token metadata to be preserved, got %#v", record.Metadata["access_token"])
	}
	if record.Metadata["refresh_token"] != "refresh-token" {
		t.Fatalf("expected refresh_token metadata to be preserved, got %#v", record.Metadata["refresh_token"])
	}
	if record.Metadata["account_id"] != accountID {
		t.Fatalf("expected account_id metadata %q, got %#v", accountID, record.Metadata["account_id"])
	}
	if record.Metadata["email"] != email {
		t.Fatalf("expected email metadata %q, got %#v", email, record.Metadata["email"])
	}
	if record.Metadata["last_refresh"] != lastRefresh {
		t.Fatalf("expected last_refresh metadata %q, got %#v", lastRefresh, record.Metadata["last_refresh"])
	}
	if record.Metadata["expired"] != expireAt {
		t.Fatalf("expected expired metadata %q, got %#v", expireAt, record.Metadata["expired"])
	}
	if record.Metadata["plan_type"] != planType {
		t.Fatalf("expected plan_type metadata %q, got %#v", planType, record.Metadata["plan_type"])
	}
	if got := record.Attributes[managedStoreAttribute]; got != "true" {
		t.Fatalf("expected managed store attribute, got %q", got)
	}
	if got := record.Attributes["plan_type"]; got != planType {
		t.Fatalf("expected plan_type attribute %q, got %q", planType, got)
	}

	digest := sha256.Sum256([]byte(accountID))
	expectedFileName := codex.CredentialFileName(email, planType, hex.EncodeToString(digest[:])[:8], true)
	if record.FileName != expectedFileName {
		t.Fatalf("expected file name %q, got %q", expectedFileName, record.FileName)
	}
	if record.ID != expectedFileName {
		t.Fatalf("expected auth id %q, got %q", expectedFileName, record.ID)
	}

	storage, ok := record.Storage.(*codex.CodexTokenStorage)
	if !ok || storage == nil {
		t.Fatalf("expected codex token storage, got %#v", record.Storage)
	}
	if storage.IDToken != idToken || storage.AccessToken != "access-token" || storage.RefreshToken != "refresh-token" {
		t.Fatalf("expected token storage to retain oauth tokens, got %#v", storage)
	}
}

func testCodexIDToken(t *testing.T, email, accountID, planType string) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("failed to marshal jwt header: %v", err)
	}
	payloadBytes, err := json.Marshal(map[string]any{
		"email": email,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id":                accountID,
			"chatgpt_plan_type":                 planType,
			"chatgpt_subscription_active_until": "2026-04-01T00:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal jwt payload: %v", err)
	}

	encode := base64.RawURLEncoding.EncodeToString
	return encode(headerBytes) + "." + encode(payloadBytes) + ".signature"
}
