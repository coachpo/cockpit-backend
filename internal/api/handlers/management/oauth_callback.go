package management

import (
	"errors"
	"html"
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
	if strings.TrimSpace(result.AuthFile) != "" {
		title = "Authentication successful"
		heading = "Authentication successful"
		message = "You can close this window and return to Cockpit."
	} else if message == "" {
		message = "Authentication failed. Check Cockpit for details."
	}

	body := "<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>" + html.EscapeString(title) + "</title><script>setTimeout(function(){window.close();},5000);</script></head><body><h1>" + html.EscapeString(heading) + "</h1><p>" + html.EscapeString(message) + "</p><p>You can close this window now. It will close automatically in 5 seconds.</p></body></html>"
	c.Data(result.StatusCode, "text/html; charset=utf-8", []byte(body))
}

func (h *Handler) GetOAuthCallback(c *gin.Context) {
	errMsg := strings.TrimSpace(c.Query("error_description"))
	if errMsg == "" {
		errMsg = strings.TrimSpace(c.Query("error"))
	}
	renderOAuthCallbackHTML(c, h.completeOAuthSession(c, c.Query("state"), "", c.Query("code"), errMsg))
}

func (h *Handler) PostOAuthSessionCallback(c *gin.Context) {
	state, ok := oauthSessionStateParam(c)
	if !ok {
		return
	}

	var req oauthSessionCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid body"})
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

	result := h.completeOAuthSession(c, state, req.Provider, code, errMsg)
	if result.StatusCode != http.StatusOK {
		c.JSON(result.StatusCode, gin.H{"status": "error", "error": result.Error})
		return
	}

	response := gin.H{"status": "ok"}
	if strings.TrimSpace(result.AuthFile) != "" {
		response["auth_file"] = result.AuthFile
	}
	c.JSON(http.StatusOK, response)
}
