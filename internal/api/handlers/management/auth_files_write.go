package management

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

func (h *Handler) buildManagedAuthRecord(name string, data []byte) (*coreauth.Auth, error) {
	name = strings.TrimSpace(filepath.Base(name))
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
	if prefix, ok := metadata["prefix"].(string); ok {
		auth.Prefix = strings.TrimSpace(prefix)
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
		return err
	}
	_, err := h.authManager.Register(ctx, auth)
	return err
}

// Upload auth file as raw JSON with ?name=
func (h *Handler) UploadAuthFile(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}
	ctx := c.Request.Context()
	name := c.Query("name")
	if name == "" || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		c.JSON(400, gin.H{"error": "invalid name"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		c.JSON(400, gin.H{"error": "name must end with .json"})
		return
	}
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
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
	c.JSON(200, gin.H{"status": "ok"})
}

// Delete auth files: single by name or all
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
	if all := c.Query("all"); all == "true" || all == "1" || all == "*" {
		deleted := 0
		for _, auth := range h.authManager.List() {
			if auth == nil || auth.ID == "" || isRuntimeOnlyAuth(auth) || !isManagedStoredAuth(auth) {
				continue
			}
			if errDelete := h.authStore.Delete(ctx, auth.ID); errDelete != nil {
				c.JSON(500, gin.H{"error": errDelete.Error()})
				return
			}
			deleted++
			h.disableAuth(ctx, auth.ID)
		}
		c.JSON(200, gin.H{"status": "ok", "deleted": deleted})
		return
	}
	name := c.Query("name")
	if name == "" || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		c.JSON(400, gin.H{"error": "invalid name"})
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
	c.JSON(200, gin.H{"status": "ok"})
}

// PatchAuthFileStatus toggles the disabled state of an auth file
func (h *Handler) PatchAuthFileStatus(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}

	var req struct {
		Name     string `json:"name"`
		Disabled *bool  `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Disabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "disabled is required"})
		return
	}

	ctx := c.Request.Context()

	targetAuth := h.findManagedAuth(name)
	if targetAuth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	targetAuth.Disabled = *req.Disabled
	if *req.Disabled {
		targetAuth.Status = coreauth.StatusDisabled
		targetAuth.StatusMessage = "disabled via management API"
	} else {
		targetAuth.Status = coreauth.StatusActive
		targetAuth.StatusMessage = ""
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

	c.JSON(http.StatusOK, gin.H{"status": "ok", "disabled": *req.Disabled})
}

// PatchAuthFileFields updates editable fields (prefix, proxy_url, priority, note) of an auth file.
func (h *Handler) PatchAuthFileFields(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}

	var req struct {
		Name     string  `json:"name"`
		Prefix   *string `json:"prefix"`
		ProxyURL *string `json:"proxy_url"`
		Priority *int    `json:"priority"`
		Note     *string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	ctx := c.Request.Context()

	targetAuth := h.findManagedAuth(name)
	if targetAuth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	changed := false
	if req.Prefix != nil {
		targetAuth.Prefix = *req.Prefix
		changed = true
	}
	if req.ProxyURL != nil {
		targetAuth.ProxyURL = *req.ProxyURL
		changed = true
	}
	if req.Priority != nil || req.Note != nil {
		if targetAuth.Metadata == nil {
			targetAuth.Metadata = make(map[string]any)
		}
		if targetAuth.Attributes == nil {
			targetAuth.Attributes = make(map[string]string)
		}

		if req.Priority != nil {
			if *req.Priority == 0 {
				delete(targetAuth.Metadata, "priority")
				delete(targetAuth.Attributes, "priority")
			} else {
				targetAuth.Metadata["priority"] = *req.Priority
				targetAuth.Attributes["priority"] = strconv.Itoa(*req.Priority)
			}
		}
		if req.Note != nil {
			trimmedNote := strings.TrimSpace(*req.Note)
			if trimmedNote == "" {
				delete(targetAuth.Metadata, "note")
				delete(targetAuth.Attributes, "note")
			} else {
				targetAuth.Metadata["note"] = trimmedNote
				targetAuth.Attributes["note"] = trimmedNote
			}
		}
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
		if auth.Attributes != nil {
			delete(auth.Attributes, "path")
			delete(auth.Attributes, "source")
		}
		_, _ = h.authManager.Update(coreauth.WithSkipPersist(ctx), auth)
	}
}
