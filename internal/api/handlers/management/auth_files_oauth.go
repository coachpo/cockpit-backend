package management

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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

const codexCallbackPort = 1455

type callbackForwarder struct {
	server *http.Server
	done   chan struct{}
}

var (
	callbackForwardersMu     sync.Mutex
	callbackForwarders       = make(map[int]*callbackForwarder)
	startOAuthCallbackServer = startCallbackForwarder
)

type oauthSessionCreateRequest struct {
	Provider string `json:"provider"`
}

func buildBackendCallbackURL(origin *url.URL) (string, error) {
	if origin == nil {
		return "", fmt.Errorf("request origin is unavailable")
	}
	if strings.TrimSpace(origin.Scheme) == "" || strings.TrimSpace(origin.Host) == "" {
		return "", fmt.Errorf("request origin is invalid")
	}

	callbackURL := *origin
	if isLoopbackHost(callbackURL.Hostname()) {
		port := callbackURL.Port()
		callbackURL.Host = "localhost"
		if port != "" {
			callbackURL.Host = net.JoinHostPort(callbackURL.Host, port)
		}
	}
	callbackURL.Path = "/auth/callback"
	callbackURL.RawPath = ""
	callbackURL.RawQuery = ""
	callbackURL.Fragment = ""
	return callbackURL.String(), nil
}

func startCallbackForwarder(port int, targetBase string) (*callbackForwarder, error) {
	callbackForwardersMu.Lock()
	prev := callbackForwarders[port]
	if prev != nil {
		delete(callbackForwarders, port)
	}
	callbackForwardersMu.Unlock()
	if prev != nil {
		stopCallbackForwarderInstance(port, prev)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on callback port %d: %w", port, err)
	}

	forwarder := &callbackForwarder{done: make(chan struct{})}
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := targetBase
		if rawQuery := strings.TrimSpace(r.URL.RawQuery); rawQuery != "" {
			if strings.Contains(target, "?") {
				target += "&" + rawQuery
			} else {
				target += "?" + rawQuery
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusFound)
		go stopCallbackForwarderInstance(port, forwarder)
	})
	forwarder.server = server

	go func() {
		if errServe := server.Serve(listener); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
		}
		close(forwarder.done)
	}()

	go func() {
		select {
		case <-time.After(oauthSessionTTL):
			stopCallbackForwarderInstance(port, forwarder)
		case <-forwarder.done:
		}
	}()

	callbackForwardersMu.Lock()
	callbackForwarders[port] = forwarder
	callbackForwardersMu.Unlock()

	return forwarder, nil
}

func stopCallbackForwarderInstance(port int, forwarder *callbackForwarder) {
	if forwarder == nil || forwarder.server == nil {
		return
	}

	callbackForwardersMu.Lock()
	if current := callbackForwarders[port]; current == forwarder {
		delete(callbackForwarders, port)
	}
	callbackForwardersMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = forwarder.server.Shutdown(ctx)
	select {
	case <-forwarder.done:
	case <-time.After(2 * time.Second):
	}
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
	requestOrigin, err := requestPublicOrigin(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	backendCallbackURL, err := buildBackendCallbackURL(requestOrigin)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	forwarder, err := startOAuthCallbackServer(codexCallbackPort, backendCallbackURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start local callback listener"})
		return
	}

	pkceCodes, err := codex.GeneratePKCECodes()
	if err != nil {
		stopCallbackForwarderInstance(codexCallbackPort, forwarder)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate PKCE codes"})
		return
	}
	state, err := misc.GenerateRandomState()
	if err != nil {
		stopCallbackForwarderInstance(codexCallbackPort, forwarder)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state parameter"})
		return
	}

	authClient := newCodexOAuthClient(h.cfg)
	authURL, err := authClient.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		stopCallbackForwarderInstance(codexCallbackPort, forwarder)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authorization url"})
		return
	}
	if err := RegisterOAuthSessionWithRedirect(state, provider, codex.RedirectURI, pkceCodes); err != nil {
		stopCallbackForwarderInstance(codexCallbackPort, forwarder)
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
