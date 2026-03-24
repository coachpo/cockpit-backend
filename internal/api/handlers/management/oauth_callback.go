package management

import (
	"errors"
	"html"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type oauthSessionCallbackRequest struct {
	Provider         string `json:"provider"`
	State            string `json:"state"`
	Code             string `json:"code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type oauthSessionCompletionResult struct {
	StatusCode int
	Provider   string
	State      string
	AuthFile   string
	Error      string
}

func normalizeOAuthCallbackProvider(provider string) (string, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "", nil
	}
	return NormalizeOAuthProvider(provider)
}

func (h *Handler) completeOAuthSession(c *gin.Context, state, provider, code, errMsg string) oauthSessionCompletionResult {
	if h == nil || h.cfg == nil {
		return oauthSessionCompletionResult{StatusCode: http.StatusInternalServerError, Error: "handler not initialized"}
	}

	state = strings.TrimSpace(state)
	if state == "" {
		return oauthSessionCompletionResult{StatusCode: http.StatusBadRequest, Error: "state is required"}
	}
	if err := ValidateOAuthState(state); err != nil {
		return oauthSessionCompletionResult{StatusCode: http.StatusBadRequest, State: state, Error: "invalid state"}
	}

	normalizedProvider, err := normalizeOAuthCallbackProvider(provider)
	if err != nil {
		return oauthSessionCompletionResult{StatusCode: http.StatusBadRequest, State: state, Error: "unsupported provider"}
	}

	code = strings.TrimSpace(code)
	errMsg = strings.TrimSpace(errMsg)
	if code == "" && errMsg == "" {
		return oauthSessionCompletionResult{StatusCode: http.StatusBadRequest, State: state, Error: "code or error is required"}
	}

	session, err := LoadOAuthSession(state)
	if err != nil {
		switch {
		case errors.Is(err, errOAuthSessionNotFound):
			return oauthSessionCompletionResult{StatusCode: http.StatusNotFound, State: state, Error: "oauth session not found"}
		case errors.Is(err, errOAuthSessionExpired):
			return oauthSessionCompletionResult{StatusCode: http.StatusGone, State: state, Error: "oauth session expired"}
		default:
			return oauthSessionCompletionResult{StatusCode: http.StatusBadRequest, State: state, Error: "invalid state"}
		}
	}

	result := oauthSessionCompletionResult{StatusCode: http.StatusOK, Provider: session.Provider, State: state}
	if session.Status != oauthStatusPending {
		result.StatusCode = http.StatusConflict
		result.Error = "oauth flow is not pending"
		return result
	}
	if normalizedProvider != "" && !strings.EqualFold(session.Provider, normalizedProvider) {
		result.StatusCode = http.StatusBadRequest
		result.Error = "provider does not match state"
		return result
	}
	if strings.TrimSpace(session.RedirectURI) == "" || session.PKCECodes == nil {
		result.StatusCode = http.StatusConflict
		result.Error = "oauth session is incomplete"
		return result
	}

	if errMsg != "" {
		_ = SetOAuthSessionError(state, errMsg)
		result.Error = errMsg
		return result
	}

	authClient := newCodexOAuthClient(h.cfg)
	bundle, errExchange := authClient.ExchangeCodeForTokensWithRedirect(PopulateAuthContext(c.Request.Context(), c), code, session.RedirectURI, session.PKCECodes)
	if errExchange != nil {
		result.StatusCode = http.StatusBadGateway
		result.Error = "Failed to exchange authorization code for tokens"
		_ = SetOAuthSessionError(state, result.Error)
		return result
	}

	record := buildCodexOAuthRecord(bundle)
	if _, errSave := h.saveTokenRecord(c.Request.Context(), record); errSave != nil {
		result.StatusCode = http.StatusInternalServerError
		result.Error = "Failed to save authentication tokens"
		_ = SetOAuthSessionError(state, result.Error)
		return result
	}
	if errUpsert := h.upsertManagedAuth(c.Request.Context(), record); errUpsert != nil {
		result.StatusCode = http.StatusInternalServerError
		result.Error = "Failed to activate authentication tokens"
		_ = SetOAuthSessionError(state, result.Error)
		return result
	}
	if errComplete := CompleteOAuthSessionWithAuthFile(state, record.FileName); errComplete != nil {
		result.StatusCode = http.StatusInternalServerError
		result.Error = "failed to finalize oauth session"
		return result
	}

	result.AuthFile = record.FileName
	return result
}

func renderOAuthCallbackHTML(c *gin.Context, result oauthSessionCompletionResult) {
	title := "Authentication failed"
	heading := "Authentication failed"
	message := strings.TrimSpace(result.Error)
	closeScript := ""
	if strings.TrimSpace(result.AuthFile) != "" {
		title = "Authentication successful"
		heading = "Authentication successful"
		message = "Return to Cockpit. This window is closing."
		closeScript = "<script>setTimeout(function(){window.close();},750);</script>"
	} else if message == "" {
		message = "Authentication failed. Check Cockpit for details."
	}

	body := "<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>" + html.EscapeString(title) + "</title>" + closeScript + "</head><body><h1>" + html.EscapeString(heading) + "</h1><p>" + html.EscapeString(message) + "</p></body></html>"
	c.Data(result.StatusCode, "text/html; charset=utf-8", []byte(body))
}

func (h *Handler) GetOAuthCallback(c *gin.Context) {
	errMsg := strings.TrimSpace(c.Query("error_description"))
	if errMsg == "" {
		errMsg = strings.TrimSpace(c.Query("error"))
	}
	renderOAuthCallbackHTML(c, h.completeOAuthSession(c, c.Query("state"), "", c.Query("code"), errMsg))
}
