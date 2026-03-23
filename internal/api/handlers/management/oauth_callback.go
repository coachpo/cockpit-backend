package management

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type oauthSessionCallbackRequest struct {
	Provider         string `json:"provider"`
	State            string `json:"state"`
	RedirectURL      string `json:"redirect_url"`
	Code             string `json:"code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (h *Handler) PostOAuthSessionCallback(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "handler not initialized"})
		return
	}

	state, ok := oauthSessionStateParam(c)
	if !ok {
		return
	}

	var req oauthSessionCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid body"})
		return
	}

	provider, err := NormalizeOAuthProvider(req.Provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "unsupported provider"})
		return
	}

	bodyState := strings.TrimSpace(req.State)
	code := strings.TrimSpace(req.Code)
	errMsg := strings.TrimSpace(req.ErrorDescription)
	if errMsg == "" {
		errMsg = strings.TrimSpace(req.Error)
	}

	if rawRedirect := strings.TrimSpace(req.RedirectURL); rawRedirect != "" {
		parsed, errParse := url.Parse(rawRedirect)
		if errParse != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid redirect_url"})
			return
		}
		query := parsed.Query()
		if bodyState == "" {
			bodyState = strings.TrimSpace(query.Get("state"))
		}
		if code == "" {
			code = strings.TrimSpace(query.Get("code"))
		}
		if errMsg == "" {
			errMsg = strings.TrimSpace(query.Get("error_description"))
			if errMsg == "" {
				errMsg = strings.TrimSpace(query.Get("error"))
			}
		}
	}

	if bodyState != "" && bodyState != state {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "state does not match resource"})
		return
	}
	if code == "" && errMsg == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "code or error is required"})
		return
	}

	session, err := LoadOAuthSession(state)
	if err != nil {
		switch {
		case errors.Is(err, errOAuthSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "oauth session not found"})
		case errors.Is(err, errOAuthSessionExpired):
			c.JSON(http.StatusGone, gin.H{"status": "error", "error": "oauth session expired"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid state"})
		}
		return
	}
	if session.Status != oauthStatusPending {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "oauth flow is not pending"})
		return
	}
	if !strings.EqualFold(session.Provider, provider) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "provider does not match state"})
		return
	}
	if strings.TrimSpace(session.RedirectURI) == "" || session.PKCECodes == nil {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "oauth session is incomplete"})
		return
	}

	if errMsg != "" {
		_ = SetOAuthSessionError(state, errMsg)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}

	authClient := newCodexOAuthClient(h.cfg)
	bundle, errExchange := authClient.ExchangeCodeForTokensWithRedirect(PopulateAuthContext(c.Request.Context(), c), code, session.RedirectURI, session.PKCECodes)
	if errExchange != nil {
		_ = SetOAuthSessionError(state, "Failed to exchange authorization code for tokens")
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "failed to exchange authorization code for tokens"})
		return
	}

	record := buildCodexOAuthRecord(bundle)
	if _, errSave := h.saveTokenRecord(c.Request.Context(), record); errSave != nil {
		_ = SetOAuthSessionError(state, "Failed to save authentication tokens")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "failed to save authentication tokens"})
		return
	}
	if errUpsert := h.upsertManagedAuth(c.Request.Context(), record); errUpsert != nil {
		_ = SetOAuthSessionError(state, "Failed to activate authentication tokens")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "failed to activate authentication tokens"})
		return
	}
	if errComplete := CompleteOAuthSessionWithAuthFile(state, record.FileName); errComplete != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "failed to finalize oauth session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "auth_file": record.FileName})
}
