package management

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

type fakeCodexOAuthClient struct {
	authURL             string
	authState           string
	authRedirectURI     string
	exchangeCode        string
	exchangeRedirectURI string
	bundle              *codex.CodexAuthBundle
	exchangeErr         error
}

func (f *fakeCodexOAuthClient) GenerateAuthURL(state string, pkceCodes *codex.PKCECodes) (string, error) {
	return f.GenerateAuthURLWithRedirect(state, codex.RedirectURI, pkceCodes)
}

func (f *fakeCodexOAuthClient) GenerateAuthURLWithRedirect(state, redirectURI string, pkceCodes *codex.PKCECodes) (string, error) {
	f.authState = state
	f.authRedirectURI = redirectURI
	if f.authURL != "" {
		return f.authURL, nil
	}
	return "https://auth.example.test/authorize?state=" + url.QueryEscape(state) + "&redirect_uri=" + url.QueryEscape(redirectURI), nil
}

func (f *fakeCodexOAuthClient) ExchangeCodeForTokens(ctx context.Context, code string, pkceCodes *codex.PKCECodes) (*codex.CodexAuthBundle, error) {
	return f.ExchangeCodeForTokensWithRedirect(ctx, code, codex.RedirectURI, pkceCodes)
}

func (f *fakeCodexOAuthClient) ExchangeCodeForTokensWithRedirect(ctx context.Context, code, redirectURI string, pkceCodes *codex.PKCECodes) (*codex.CodexAuthBundle, error) {
	f.exchangeCode = code
	f.exchangeRedirectURI = redirectURI
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return f.bundle, nil
}

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

func TestCreateOAuthSession_UsesFrontendCallbackRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStore := oauthSessions
	oauthSessions = newOAuthSessionStore(oauthSessionTTL)
	defer func() { oauthSessions = oldStore }()

	h := NewHandlerWithoutConfigFilePath(&config.Config{}, nil)
	h.authStore = &readonlyAuthStore{}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8317/v0/management/oauth-sessions", strings.NewReader(`{"provider":"codex","callback_origin":"http://127.0.0.1:5173"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.CreateOAuthSession(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var body struct {
		Status string `json:"status"`
		URL    string `json:"url"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if body.Status != "ok" || body.State == "" || body.URL == "" {
		t.Fatalf("expected create response with status/url/state, got %+v", body)
	}
	parsed, err := url.Parse(body.URL)
	if err != nil {
		t.Fatalf("failed to parse auth url: %v", err)
	}
	if got := parsed.Query().Get("redirect_uri"); got != "http://127.0.0.1:5173/codex/callback" {
		t.Fatalf("expected redirect_uri to target frontend callback page, got %q", got)
	}
	session, err := LoadOAuthSession(body.State)
	if err != nil {
		t.Fatalf("expected stored oauth session, got %v", err)
	}
	if session.Status != oauthStatusPending || session.RedirectURI != "http://127.0.0.1:5173/codex/callback" || session.Provider != "codex" {
		t.Fatalf("unexpected stored session: %+v", session)
	}
}

func TestCreateOAuthSession_RejectsCallbackOutsideFrontendOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandlerWithoutConfigFilePath(&config.Config{}, nil)
	h.authStore = &readonlyAuthStore{}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "https://cockpit.example.com/v0/management/oauth-sessions", strings.NewReader(`{"provider":"codex","callback_origin":"https://evil.example"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	ctx.Request.Header.Set("X-Forwarded-Host", "cockpit.example.com")

	h.CreateOAuthSession(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "callback_origin must match the current frontend origin") {
		t.Fatalf("expected callback origin validation error, got %s", rec.Body.String())
	}
}

func TestGetOAuthSessionStatus_ReturnsGoneForExpiredSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStore := oauthSessions
	oauthSessions = newOAuthSessionStore(time.Nanosecond)
	defer func() { oauthSessions = oldStore }()

	if err := RegisterOAuthSession("expired-state", "codex"); err != nil {
		t.Fatalf("failed to register oauth session: %v", err)
	}
	time.Sleep(time.Millisecond)

	h := NewHandlerWithoutConfigFilePath(&config.Config{}, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/oauth-sessions/expired-state", nil)
	ctx.Params = gin.Params{{Key: "state", Value: "expired-state"}}

	h.GetOAuthSessionStatus(ctx)

	if rec.Code != http.StatusGone {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusGone, rec.Code, rec.Body.String())
	}
}

func TestPostOAuthSessionCallback_UsesStoredRedirectURIForExchange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStore := oauthSessions
	oauthSessions = newOAuthSessionStore(oauthSessionTTL)
	defer func() { oauthSessions = oldStore }()
	oldFactory := newCodexOAuthClient
	defer func() { newCodexOAuthClient = oldFactory }()

	fake := &fakeCodexOAuthClient{bundle: &codex.CodexAuthBundle{TokenData: codex.CodexTokenData{
		IDToken:      testCodexIDToken(t, "oauth@example.com", "acct_123", "plus"),
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		AccountID:    "acct_123",
		Email:        "oauth@example.com",
		Expire:       "2026-03-23T19:00:00Z",
	}, LastRefresh: "2026-03-22T19:00:00Z"}}
	newCodexOAuthClient = func(cfg *config.Config) codexOAuthClient { return fake }

	if err := RegisterOAuthSessionWithRedirect("state-123", "codex", "http://127.0.0.1:5173/codex/callback", &codex.PKCECodes{CodeVerifier: "verifier", CodeChallenge: "challenge"}); err != nil {
		t.Fatalf("failed to register oauth session: %v", err)
	}

	store := &recordingAuthStore{}
	h := NewHandlerWithoutConfigFilePath(&config.Config{}, nil)
	h.authStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/oauth-sessions/state-123/callback", strings.NewReader(`{"provider":"codex","code":"auth-code"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "state", Value: "state-123"}}

	h.PostOAuthSessionCallback(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if fake.exchangeCode != "auth-code" {
		t.Fatalf("expected callback code auth-code, got %q", fake.exchangeCode)
	}
	if fake.exchangeRedirectURI != "http://127.0.0.1:5173/codex/callback" {
		t.Fatalf("expected stored redirect uri to be reused during exchange, got %q", fake.exchangeRedirectURI)
	}
	if saved := store.lastSaved(); saved == nil || strings.TrimSpace(saved.FileName) == "" {
		t.Fatalf("expected exchanged tokens to be persisted, got %#v", saved)
	}
	session, err := LoadOAuthSession("state-123")
	if err != nil {
		t.Fatalf("expected completed oauth session, got %v", err)
	}
	if session.Status != oauthStatusComplete || strings.TrimSpace(session.AuthFile) == "" {
		t.Fatalf("expected completed oauth session with auth file, got %+v", session)
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
