package management

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

type authFileUsageRequest struct {
	Method string
	URL    string
	Header map[string]string
	Body   []byte
}

func buildAuthFileUsageRequest(auth *coreauth.Auth) *authFileUsageRequest {
	if auth == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") {
		return nil
	}
	if auth.EnsureIndex() == "" {
		return nil
	}

	header := map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Content-Type":  "application/json",
		"User-Agent":    "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal",
	}
	if claims := extractCodexIDTokenClaims(auth); claims != nil {
		if accountID, ok := claims["chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
			header["Chatgpt-Account-Id"] = strings.TrimSpace(accountID)
		}
	}

	return &authFileUsageRequest{
		Method: http.MethodGet,
		URL:    "https://chatgpt.com/backend-api/wham/usage",
		Header: header,
	}
}

func (h *Handler) RefreshAuthFileUsage(c *gin.Context) {
	if h == nil || h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth manager unavailable"})
		return
	}

	name, ok := authFileNameParam(c)
	if !ok {
		return
	}

	auth := h.findManagedAuth(name)
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	usageRequest := buildAuthFileUsageRequest(auth)
	if usageRequest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usage unavailable for auth file"})
		return
	}

	headers := make(http.Header, len(usageRequest.Header))
	for key, value := range usageRequest.Header {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		headers.Set(trimmedKey, value)
	}

	req, err := h.authManager.NewHttpRequest(c.Request.Context(), auth, usageRequest.Method, usageRequest.URL, usageRequest.Body, headers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.authManager.HttpRequest(c.Request.Context(), auth, req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
		return
	}

	var decoded map[string]any
	if len(respBody) > 0 {
		if err := jsonUnmarshalObject(respBody, &decoded); err != nil {
			decoded = nil
		}
	}

	if decoded != nil {
		if err := h.persistAuthFileUsage(c.Request.Context(), auth, decoded); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}
	if decoded != nil {
		c.JSON(resp.StatusCode, decoded)
		return
	}

	c.JSON(resp.StatusCode, gin.H{
		"status": resp.StatusCode,
		"body":   string(respBody),
	})
}

func (h *Handler) persistAuthFileUsage(ctx context.Context, auth *coreauth.Auth, usage map[string]any) error {
	if auth == nil || usage == nil {
		return nil
	}
	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	auth.Metadata["usage"] = usage
	auth.UpdatedAt = time.Now()
	if h.authStore != nil {
		if _, err := h.authStore.Save(ctx, auth); err != nil {
			return err
		}
	}
	return h.upsertManagedAuth(ctx, auth)
}

func jsonUnmarshalObject(data []byte, target *map[string]any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}
