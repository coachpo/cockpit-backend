package management

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
	proxyconfig "github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/misc"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

type codexOAuthClient interface {
	GenerateAuthURL(state string, pkceCodes *codex.PKCECodes) (string, error)
	GenerateAuthURLWithRedirect(state, redirectURI string, pkceCodes *codex.PKCECodes) (string, error)
	ExchangeCodeForTokens(ctx context.Context, code string, pkceCodes *codex.PKCECodes) (*codex.CodexAuthBundle, error)
	ExchangeCodeForTokensWithRedirect(ctx context.Context, code, redirectURI string, pkceCodes *codex.PKCECodes) (*codex.CodexAuthBundle, error)
}

var newCodexOAuthClient = func(cfg *proxyconfig.Config) codexOAuthClient {
	return codex.NewCodexAuth(cfg)
}

type oauthSessionCreateRequest struct {
	Provider       string `json:"provider"`
	CallbackOrigin string `json:"callback_origin"`
}

func buildFrontendCallbackURL(callbackOrigin string) (string, error) {
	callbackOrigin = strings.TrimSpace(callbackOrigin)
	if callbackOrigin == "" {
		return "", fmt.Errorf("callback_origin is required")
	}
	parsed, err := url.Parse(callbackOrigin)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("invalid callback_origin")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("invalid callback_origin")
	}
	parsed.Path = "/codex/callback"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func firstForwardedValue(value string) string {
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func requestPublicOrigin(c *gin.Context) (*url.URL, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("request is unavailable")
	}

	scheme := strings.ToLower(firstForwardedValue(c.GetHeader("X-Forwarded-Proto")))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := firstForwardedValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return nil, fmt.Errorf("request host is unavailable")
	}

	return url.Parse(scheme + "://" + host)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeOriginKey(origin *url.URL) string {
	if origin == nil {
		return ""
	}
	host := strings.ToLower(origin.Hostname())
	if host == "" {
		return ""
	}
	port := origin.Port()
	switch {
	case strings.EqualFold(origin.Scheme, "http") && port == "80":
		port = ""
	case strings.EqualFold(origin.Scheme, "https") && port == "443":
		port = ""
	}
	if port != "" {
		return strings.ToLower(origin.Scheme) + "://" + net.JoinHostPort(host, port)
	}
	return strings.ToLower(origin.Scheme) + "://" + host
}

func validateFrontendCallbackOrigin(c *gin.Context, callbackURL string) error {
	requestOrigin, err := requestPublicOrigin(c)
	if err != nil {
		return err
	}
	callbackOrigin, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil {
		return fmt.Errorf("invalid callback_origin")
	}
	if normalizeOriginKey(callbackOrigin) == normalizeOriginKey(requestOrigin) {
		return nil
	}
	if isLoopbackHost(callbackOrigin.Hostname()) && isLoopbackHost(requestOrigin.Hostname()) {
		return nil
	}
	return fmt.Errorf("callback_origin must match the current frontend origin")
}

func oauthSessionStateParam(c *gin.Context) (string, bool) {
	state := strings.TrimSpace(c.Param("state"))
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "state is required"})
		return "", false
	}
	if err := ValidateOAuthState(state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid state"})
		return "", false
	}
	return state, true
}

func (h *Handler) CreateOAuthSession(c *gin.Context) {
	if h.authStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "auth store unavailable"})
		return
	}

	var req oauthSessionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	provider, err := NormalizeOAuthProvider(req.Provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider"})
		return
	}
	frontendCallbackURL, err := buildFrontendCallbackURL(req.CallbackOrigin)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateFrontendCallbackOrigin(c, frontendCallbackURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pkceCodes, err := codex.GeneratePKCECodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate PKCE codes"})
		return
	}
	state, err := misc.GenerateRandomState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state parameter"})
		return
	}

	authClient := newCodexOAuthClient(h.cfg)
	authURL, err := authClient.GenerateAuthURLWithRedirect(state, frontendCallbackURL, pkceCodes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authorization url"})
		return
	}
	if err := RegisterOAuthSessionWithRedirect(state, provider, frontendCallbackURL, pkceCodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register oauth session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "url": authURL, "state": state})
}

func (h *Handler) GetOAuthSessionStatus(c *gin.Context) {
	state, ok := oauthSessionStateParam(c)
	if !ok {
		return
	}
	session, err := LoadOAuthSession(state)
	if err != nil {
		switch {
		case err == errOAuthSessionNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "oauth session not found"})
		case err == errOAuthSessionExpired:
			c.JSON(http.StatusGone, gin.H{"error": "oauth session expired"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		}
		return
	}

	status := strings.TrimSpace(session.Status)
	if status == "" {
		status = oauthStatusPending
	}
	response := gin.H{
		"status":   status,
		"provider": session.Provider,
		"state":    state,
	}
	if session.Error != "" {
		response["error"] = session.Error
	}
	if session.AuthFile != "" {
		response["auth_file"] = session.AuthFile
	}
	c.JSON(http.StatusOK, response)
}

func buildCodexOAuthRecord(bundle *codex.CodexAuthBundle) *coreauth.Auth {
	if bundle == nil {
		return nil
	}

	tokenData := bundle.TokenData
	claims, _ := codex.ParseJWTToken(tokenData.IDToken)

	email := strings.TrimSpace(tokenData.Email)
	if email == "" && claims != nil {
		email = strings.TrimSpace(claims.Email)
	}
	accountID := strings.TrimSpace(tokenData.AccountID)
	if accountID == "" && claims != nil {
		accountID = strings.TrimSpace(claims.GetAccountID())
	}

	planType := ""
	hashAccountID := ""
	if claims != nil {
		planType = strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType)
		if accountID != "" {
			digest := sha256.Sum256([]byte(accountID))
			hashAccountID = hex.EncodeToString(digest[:])[:8]
		}
	}

	storage := &codex.CodexTokenStorage{
		IDToken:      tokenData.IDToken,
		AccessToken:  tokenData.AccessToken,
		RefreshToken: tokenData.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  bundle.LastRefresh,
		Email:        email,
		Expire:       tokenData.Expire,
	}

	metadata := map[string]any{}
	if email != "" {
		metadata["email"] = email
	}
	if accountID != "" {
		metadata["account_id"] = accountID
	}
	if v := strings.TrimSpace(tokenData.IDToken); v != "" {
		metadata["id_token"] = v
	}
	if v := strings.TrimSpace(tokenData.AccessToken); v != "" {
		metadata["access_token"] = v
	}
	if v := strings.TrimSpace(tokenData.RefreshToken); v != "" {
		metadata["refresh_token"] = v
	}
	if v := strings.TrimSpace(bundle.LastRefresh); v != "" {
		metadata["last_refresh"] = v
	}
	if v := strings.TrimSpace(tokenData.Expire); v != "" {
		metadata["expired"] = v
	}
	if planType != "" {
		metadata["plan_type"] = planType
	}
	storage.SetMetadata(metadata)

	attributes := map[string]string{managedStoreAttribute: "true"}
	if planType != "" {
		attributes["plan_type"] = planType
	}

	fileName := codex.CredentialFileName(email, planType, hashAccountID, true)
	return &coreauth.Auth{
		ID:         fileName,
		Provider:   "codex",
		FileName:   fileName,
		Label:      email,
		Status:     coreauth.StatusActive,
		Storage:    storage,
		Metadata:   metadata,
		Attributes: attributes,
	}
}

func (h *Handler) saveTokenRecord(ctx context.Context, record *coreauth.Auth) (string, error) {
	if record == nil {
		return "", fmt.Errorf("token record is nil")
	}
	if h.authStore == nil {
		return "", fmt.Errorf("auth store unavailable")
	}
	if h.postAuthHook != nil {
		if err := h.postAuthHook(ctx, record); err != nil {
			return "", fmt.Errorf("post-auth hook failed: %w", err)
		}
	}
	return h.authStore.Save(ctx, record)
}

func PopulateAuthContext(ctx context.Context, c *gin.Context) context.Context {
	info := &coreauth.RequestInfo{Query: c.Request.URL.Query(), Headers: c.Request.Header}
	return coreauth.WithRequestInfo(ctx, info)
}
