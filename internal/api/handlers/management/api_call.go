package management

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

type managementAPICallRequest struct {
	AuthIndex string            `json:"authIndex"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Header    map[string]string `json:"header"`
	Body      any               `json:"body"`
}

func (h *Handler) APICall(c *gin.Context) {
	if h == nil || h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth manager unavailable"})
		return
	}

	var body managementAPICallRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	method := strings.ToUpper(strings.TrimSpace(body.Method))
	url := strings.TrimSpace(body.URL)
	if method == "" || url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "method and url are required"})
		return
	}
	if strings.TrimSpace(body.AuthIndex) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "authIndex is required"})
		return
	}

	auth := h.findManagedAuthByIndex(body.AuthIndex)
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	headers := make(http.Header, len(body.Header))
	for key, value := range body.Header {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		headers.Set(trimmedKey, value)
	}

	var rawBody []byte
	if body.Body != nil {
		encoded, err := json.Marshal(body.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "body must be valid json"})
			return
		}
		if !bytes.Equal(encoded, []byte("null")) {
			rawBody = encoded
		}
	}

	req, err := h.authManager.NewHttpRequest(c.Request.Context(), auth, method, url, rawBody, headers)
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

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	var decoded any
	if len(respBody) > 0 && json.Unmarshal(respBody, &decoded) == nil {
		c.JSON(resp.StatusCode, decoded)
		return
	}

	c.JSON(resp.StatusCode, gin.H{
		"status": resp.StatusCode,
		"body":   string(respBody),
	})
}

func (h *Handler) findManagedAuthByIndex(authIndex string) *coreauth.Auth {
	if h == nil || h.authManager == nil {
		return nil
	}
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return nil
	}
	for _, auth := range h.authManager.List() {
		if auth == nil || !isManagedStoredAuth(auth) {
			continue
		}
		if auth.EnsureIndex() == authIndex {
			return auth
		}
	}
	return nil
}
