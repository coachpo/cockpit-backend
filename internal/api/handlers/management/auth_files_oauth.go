package management

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
	"github.com/coachpo/cockpit-backend/internal/misc"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const codexCallbackPort = 1455

type callbackForwarder struct {
	provider string
	server   *http.Server
	done     chan struct{}
}

var (
	callbackForwardersMu sync.Mutex
	callbackForwarders   = make(map[int]*callbackForwarder)
)

func isWebUIRequest(c *gin.Context) bool {
	raw := strings.TrimSpace(c.Query("is_webui"))
	if raw == "" {
		return false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func startCallbackForwarder(port int, provider, targetBase string) (*callbackForwarder, error) {
	callbackForwardersMu.Lock()
	prev := callbackForwarders[port]
	if prev != nil {
		delete(callbackForwarders, port)
	}
	callbackForwardersMu.Unlock()
	if prev != nil {
		stopForwarderInstance(port, prev)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := targetBase
		if raw := r.URL.RawQuery; raw != "" {
			if strings.Contains(target, "?") {
				target = target + "&" + raw
			} else {
				target = target + "?" + raw
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusFound)
	})
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	done := make(chan struct{})
	go func() {
		if errServe := srv.Serve(ln); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
			log.WithError(errServe).Warnf("callback forwarder for %s stopped unexpectedly", provider)
		}
		close(done)
	}()
	forwarder := &callbackForwarder{provider: provider, server: srv, done: done}
	callbackForwardersMu.Lock()
	callbackForwarders[port] = forwarder
	callbackForwardersMu.Unlock()
	log.Infof("callback forwarder for %s listening on %s", provider, addr)
	return forwarder, nil
}

func stopCallbackForwarderInstance(port int, forwarder *callbackForwarder) {
	if forwarder == nil {
		return
	}
	callbackForwardersMu.Lock()
	if current := callbackForwarders[port]; current == forwarder {
		delete(callbackForwarders, port)
	}
	callbackForwardersMu.Unlock()
	stopForwarderInstance(port, forwarder)
}

func stopForwarderInstance(port int, forwarder *callbackForwarder) {
	if forwarder == nil || forwarder.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := forwarder.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.WithError(err).Warnf("failed to shut down callback forwarder on port %d", port)
	}
	select {
	case <-forwarder.done:
	case <-time.After(2 * time.Second):
	}
	log.Infof("callback forwarder on port %d stopped", port)
}

func (h *Handler) managementCallbackURL(path string) (string, error) {
	if h == nil || h.cfg == nil || h.cfg.Port <= 0 {
		return "", fmt.Errorf("server port is not configured")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", h.cfg.Port, path), nil
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

func (h *Handler) RequestCodexToken(c *gin.Context) {
	if h.authStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "auth store unavailable"})
		return
	}
	ctx := PopulateAuthContext(context.Background(), c)
	pkceCodes, err := codex.GeneratePKCECodes()
	if err != nil {
		log.Errorf("Failed to generate PKCE codes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate PKCE codes"})
		return
	}
	state, err := misc.GenerateRandomState()
	if err != nil {
		log.Errorf("Failed to generate state parameter: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state parameter"})
		return
	}
	openaiAuth := codex.NewCodexAuth(h.cfg)
	authURL, err := openaiAuth.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		log.Errorf("Failed to generate authorization URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authorization url"})
		return
	}
	RegisterOAuthSession(state, "codex")
	isWebUI := isWebUIRequest(c)
	var forwarder *callbackForwarder
	if isWebUI {
		targetURL, errTarget := h.managementCallbackURL("/codex/callback")
		if errTarget != nil {
			log.WithError(errTarget).Error("failed to compute codex callback target")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "callback server unavailable"})
			return
		}
		if forwarder, err = startCallbackForwarder(codexCallbackPort, "codex", targetURL); err != nil {
			log.WithError(err).Error("failed to start codex callback forwarder")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start callback server"})
			return
		}
	}
	go h.completeCodexOAuthFlow(ctx, state, pkceCodes, openaiAuth, isWebUI, forwarder)
	c.JSON(200, gin.H{"status": "ok", "url": authURL, "state": state})
}

func (h *Handler) completeCodexOAuthFlow(ctx context.Context, state string, pkceCodes *codex.PKCECodes, openaiAuth *codex.CodexAuth, isWebUI bool, forwarder *callbackForwarder) {
	if isWebUI {
		defer stopCallbackForwarderInstance(codexCallbackPort, forwarder)
	}
	deadline := time.Now().Add(5 * time.Minute)
	var code string
	for {
		if !IsOAuthSessionPending(state, "codex") {
			return
		}
		if time.Now().After(deadline) {
			authErr := codex.NewAuthenticationError(codex.ErrCallbackTimeout, fmt.Errorf("timeout waiting for OAuth callback"))
			log.Error(codex.GetUserFriendlyMessage(authErr))
			SetOAuthSessionError(state, "Timeout waiting for OAuth callback")
			return
		}
		payload, ready, errConsume := consumeOAuthCallback(state, "codex")
		if errConsume != nil {
			SetOAuthSessionError(state, "OAuth callback unavailable")
			return
		}
		if ready {
			if errStr := payload.Error; errStr != "" {
				oauthErr := codex.NewOAuthError(errStr, "", http.StatusBadRequest)
				log.Error(codex.GetUserFriendlyMessage(oauthErr))
				SetOAuthSessionError(state, "Bad Request")
				return
			}
			if payload.State != state {
				authErr := codex.NewAuthenticationError(codex.ErrInvalidState, fmt.Errorf("expected %s, got %s", state, payload.State))
				SetOAuthSessionError(state, "State code error")
				log.Error(codex.GetUserFriendlyMessage(authErr))
				return
			}
			code = payload.Code
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	bundle, errExchange := openaiAuth.ExchangeCodeForTokens(ctx, code, pkceCodes)
	if errExchange != nil {
		authErr := codex.NewAuthenticationError(codex.ErrCodeExchangeFailed, errExchange)
		SetOAuthSessionError(state, "Failed to exchange authorization code for tokens")
		log.Errorf("Failed to exchange authorization code for tokens: %v", authErr)
		return
	}
	claims, _ := codex.ParseJWTToken(bundle.TokenData.IDToken)
	planType := ""
	hashAccountID := ""
	if claims != nil {
		planType = strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType)
		if accountID := claims.GetAccountID(); accountID != "" {
			digest := sha256.Sum256([]byte(accountID))
			hashAccountID = hex.EncodeToString(digest[:])[:8]
		}
	}
	tokenStorage := openaiAuth.CreateTokenStorage(bundle)
	fileName := codex.CredentialFileName(tokenStorage.Email, planType, hashAccountID, true)
	record := &coreauth.Auth{ID: fileName, Provider: "codex", FileName: fileName, Storage: tokenStorage, Metadata: map[string]any{"email": tokenStorage.Email, "account_id": tokenStorage.AccountID}, Attributes: map[string]string{managedStoreAttribute: "true"}}
	savedPath, errSave := h.saveTokenRecord(ctx, record)
	if errSave != nil {
		SetOAuthSessionError(state, "Failed to save authentication tokens")
		log.Errorf("Failed to save authentication tokens: %v", errSave)
		return
	}
	fmt.Printf("Authentication successful! Token saved to %s\n", savedPath)
	if bundle.APIKey != "" {
		fmt.Println("API key obtained and saved")
	}
	fmt.Println("You can now use Codex services through this CLI")
	CompleteOAuthSession(state)
	CompleteOAuthSessionsByProvider("codex")
}

func (h *Handler) GetAuthStatus(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	if state == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if err := ValidateOAuthState(state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid state"})
		return
	}
	_, status, ok := GetOAuthSession(state)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if status != "" {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": status})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "wait"})
}

func PopulateAuthContext(ctx context.Context, c *gin.Context) context.Context {
	info := &coreauth.RequestInfo{Query: c.Request.URL.Query(), Headers: c.Request.Header}
	return coreauth.WithRequestInfo(ctx, info)
}
