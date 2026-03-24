package management

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
	"github.com/gin-gonic/gin"
)

type createAuthFileRequest struct {
	Name    string         `json:"name"`
	Content map[string]any `json:"content"`
}

type patchAuthFileRequest struct {
	Disabled *bool `json:"disabled"`
	Priority *int  `json:"priority"`
}

func (h *Handler) buildManagedAuthRecord(name string, data []byte) (*coreauth.Auth, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("invalid name")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("auth payload is empty")
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("invalid auth file: %w", err)
	}
	provider, _ := metadata["type"].(string)
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "unknown"
	}
	label := provider
	if email, ok := metadata["email"].(string); ok && strings.TrimSpace(email) != "" {
		label = strings.TrimSpace(email)
	}
	now := time.Now()
	lastRefresh, hasLastRefresh := extractLastRefreshTimestamp(metadata)
	existing := h.findManagedAuth(name)
	authID := authIDForName(name)
	if existing != nil && strings.TrimSpace(existing.ID) != "" {
		authID = strings.TrimSpace(existing.ID)
	}
	auth := &coreauth.Auth{
		ID:         authID,
		Provider:   provider,
		FileName:   name,
		Label:      label,
		Status:     coreauth.StatusActive,
		Attributes: buildAuthAttributes(metadata),
		Metadata:   metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if proxyURL, ok := metadata["proxy_url"].(string); ok {
		auth.ProxyURL = strings.TrimSpace(proxyURL)
	}
	if disabled, ok := metadata["disabled"].(bool); ok && disabled {
		auth.Disabled = true
		auth.Status = coreauth.StatusDisabled
		auth.StatusMessage = "disabled via management API"
	}
	if hasLastRefresh {
		auth.LastRefreshedAt = lastRefresh
	}
	if existing != nil {
		auth.CreatedAt = existing.CreatedAt
		if !hasLastRefresh {
			auth.LastRefreshedAt = existing.LastRefreshedAt
		}
		auth.NextRefreshAfter = existing.NextRefreshAfter
		auth.NextRetryAfter = existing.NextRetryAfter
		auth.Runtime = existing.Runtime
		auth.ModelStates = existing.ModelStates
	}
	return auth, nil
}

func (h *Handler) upsertManagedAuth(ctx context.Context, auth *coreauth.Auth) error {
	if h == nil || h.authManager == nil || auth == nil {
		return nil
	}
	ctx = coreauth.WithSkipPersist(ctx)
	if existing, ok := h.authManager.GetByID(auth.ID); ok {
		auth.CreatedAt = existing.CreatedAt
		if auth.LastRefreshedAt.IsZero() {
			auth.LastRefreshedAt = existing.LastRefreshedAt
		}
		auth.NextRefreshAfter = existing.NextRefreshAfter
		auth.NextRetryAfter = existing.NextRetryAfter
		if len(auth.ModelStates) == 0 {
			auth.ModelStates = existing.ModelStates
		}
		auth.Runtime = existing.Runtime
		_, err := h.authManager.Update(ctx, auth)
		if err == nil {
			syncManagedAuthModels(auth)
		}
		return err
	}
	_, err := h.authManager.Register(ctx, auth)
	if err == nil {
		syncManagedAuthModels(auth)
	}
	return err
}

func (h *Handler) CreateAuthFile(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}

	var req createAuthFileRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" || req.Content == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		c.JSON(400, gin.H{"error": "invalid name"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		c.JSON(400, gin.H{"error": "name must end with .json"})
		return
	}
	data, err := json.Marshal(req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must be valid json"})
		return
	}

	record, errBuild := h.buildManagedAuthRecord(name, data)
	if errBuild != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errBuild.Error()})
		return
	}
	if _, errSave := h.authStore.Save(ctx, record); errSave != nil {
		c.JSON(500, gin.H{"error": errSave.Error()})
		return
	}
	if errUpsert := h.upsertManagedAuth(ctx, record); errUpsert != nil {
		c.JSON(500, gin.H{"error": errUpsert.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok", "name": name})
}

func (h *Handler) DeleteAuthFile(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}
	ctx := c.Request.Context()
	name, ok := authFileNameParam(c)
	if !ok {
		return
	}

	targetAuth := h.findManagedAuth(name)
	if targetAuth == nil {
		c.JSON(404, gin.H{"error": "auth file not found"})
		return
	}
	if errDeleteRecord := h.authStore.Delete(ctx, targetAuth.ID); errDeleteRecord != nil {
		c.JSON(500, gin.H{"error": errDeleteRecord.Error()})
		return
	}
	h.disableAuth(ctx, targetAuth.ID)
	resolvedName := strings.TrimSpace(targetAuth.FileName)
	if resolvedName == "" {
		resolvedName = name
	}
	c.JSON(200, gin.H{"status": "ok", "name": resolvedName})
}

func (h *Handler) PatchAuthFile(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}

	name, ok := authFileNameParam(c)
	if !ok {
		return
	}

	var req patchAuthFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	targetAuth := h.findManagedAuth(name)
	if targetAuth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	changed := false
	if req.Disabled != nil {
		targetAuth.Disabled = *req.Disabled
		if *req.Disabled {
			targetAuth.Status = coreauth.StatusDisabled
			targetAuth.StatusMessage = "disabled via management API"
		} else {
			targetAuth.Status = coreauth.StatusActive
			targetAuth.StatusMessage = ""
		}
		changed = true
	}
	if req.Priority != nil {
		if targetAuth.Metadata == nil {
			targetAuth.Metadata = make(map[string]any)
		}
		if targetAuth.Attributes == nil {
			targetAuth.Attributes = make(map[string]string)
		}
		targetAuth.Metadata["priority"] = *req.Priority
		targetAuth.Attributes["priority"] = strconv.Itoa(*req.Priority)
		changed = true
	}

	if !changed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	targetAuth.UpdatedAt = time.Now()

	if _, err := h.authStore.Save(ctx, targetAuth); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.upsertManagedAuth(ctx, targetAuth); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update auth: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) disableAuth(ctx context.Context, id string) {
	if h == nil || h.authManager == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if auth, ok := h.authManager.GetByID(id); ok {
		auth.Disabled = true
		auth.Status = coreauth.StatusDisabled
		auth.StatusMessage = "removed via management API"
		auth.UpdatedAt = time.Now()
		_, _ = h.authManager.Update(coreauth.WithSkipPersist(ctx), auth)
		syncManagedAuthModels(auth)
	}
}
