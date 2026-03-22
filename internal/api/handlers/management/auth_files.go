package management

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"github.com/coachpo/cockpit-backend/internal/registry"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func (h *Handler) ListAuthFiles(c *gin.Context) {
	if h == nil {
		c.JSON(500, gin.H{"error": "handler not initialized"})
		return
	}
	if h.authManager == nil {
		if h.authStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
			return
		}
		h.listAuthFilesFromStore(c)
		return
	}
	metadataByName := map[string]nacos.AuthFileMetadata{}
	if h.authStore != nil {
		metadataByName = h.authMetadataByName(c.Request.Context())
	}
	auths := h.authManager.List()
	files := make([]gin.H, 0, len(auths))
	for _, auth := range auths {
		if entry := h.buildAuthFileEntry(auth, metadataByName); entry != nil {
			files = append(files, entry)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		nameI, _ := files[i]["name"].(string)
		nameJ, _ := files[j]["name"].(string)
		return strings.ToLower(nameI) < strings.ToLower(nameJ)
	})
	c.JSON(200, gin.H{"files": files})
}

// GetAuthFileModels returns the models supported by a specific auth file
func (h *Handler) GetAuthFileModels(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.JSON(400, gin.H{"error": "name is required"})
		return
	}

	targetAuth := h.findManagedAuth(name)
	if targetAuth == nil {
		c.JSON(404, gin.H{"error": "auth file not found"})
		return
	}

	// Get models from registry
	reg := registry.GetGlobalRegistry()
	models := reg.GetModelsForClient(targetAuth.ID)

	result := make([]gin.H, 0, len(models))
	for _, m := range models {
		entry := gin.H{
			"id": m.ID,
		}
		if m.DisplayName != "" {
			entry["display_name"] = m.DisplayName
		}
		if m.Type != "" {
			entry["type"] = m.Type
		}
		if m.OwnedBy != "" {
			entry["owned_by"] = m.OwnedBy
		}
		result = append(result, entry)
	}

	c.JSON(200, gin.H{"models": result})
}

func (h *Handler) listAuthFilesFromStore(c *gin.Context) {
	items, err := h.authStore.ListMetadata(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to list auth metadata: %v", err)})
		return
	}
	files := make([]gin.H, 0, len(items))
	for _, item := range items {
		entry := gin.H{
			"id":     item.ID,
			"name":   item.Name,
			"type":   item.Type,
			"email":  item.Email,
			"size":   item.Size,
			"source": item.Source,
		}
		if !item.ModTime.IsZero() {
			entry["modtime"] = item.ModTime
		}
		if item.Priority != nil {
			entry["priority"] = *item.Priority
		}
		if item.Note != "" {
			entry["note"] = item.Note
		}
		files = append(files, entry)
	}
	c.JSON(200, gin.H{"files": files})
}

func (h *Handler) authMetadataByName(ctx context.Context) map[string]nacos.AuthFileMetadata {
	items, err := h.authStore.ListMetadata(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to load auth metadata from store")
		return nil
	}
	result := make(map[string]nacos.AuthFileMetadata, len(items))
	for _, item := range items {
		result[strings.TrimSpace(item.Name)] = item
		if item.ID != "" {
			result[item.ID] = item
		}
	}
	return result
}

func (h *Handler) buildAuthFileEntry(auth *coreauth.Auth, metadataByName map[string]nacos.AuthFileMetadata) gin.H {
	if auth == nil {
		return nil
	}
	if !isManagedStoredAuth(auth) {
		return nil
	}
	auth.EnsureIndex()
	runtimeOnly := isRuntimeOnlyAuth(auth)
	removedByManagement := !runtimeOnly && (auth.Disabled || auth.Status == coreauth.StatusDisabled) && strings.EqualFold(strings.TrimSpace(auth.StatusMessage), "removed via management API")
	if runtimeOnly && (auth.Disabled || auth.Status == coreauth.StatusDisabled) {
		return nil
	}
	if removedByManagement {
		return nil
	}
	name := strings.TrimSpace(auth.FileName)
	if name == "" {
		name = auth.ID
	}
	status := strings.TrimSpace(string(auth.Status))
	if status == "" {
		if auth.Disabled {
			status = string(coreauth.StatusDisabled)
		} else {
			status = string(coreauth.StatusActive)
		}
	}
	entry := gin.H{
		"id":             auth.ID,
		"auth_index":     auth.Index,
		"name":           name,
		"type":           strings.TrimSpace(auth.Provider),
		"provider":       strings.TrimSpace(auth.Provider),
		"label":          auth.Label,
		"status":         status,
		"status_message": auth.StatusMessage,
		"disabled":       auth.Disabled,
		"unavailable":    auth.Unavailable,
		"runtime_only":   runtimeOnly,
		"source":         "memory",
		"size":           int64(0),
	}
	if item, ok := metadataByName[name]; ok {
		entry["source"] = item.Source
		entry["size"] = item.Size
		if !item.ModTime.IsZero() {
			entry["modtime"] = item.ModTime
		}
	}
	if email := authEmail(auth); email != "" {
		entry["email"] = email
	}
	if accountType, account := auth.AccountInfo(); accountType != "" || account != "" {
		if accountType != "" {
			entry["account_type"] = accountType
		}
		if account != "" {
			entry["account"] = account
		}
	}
	if !auth.CreatedAt.IsZero() {
		entry["created_at"] = auth.CreatedAt
	}
	if !auth.UpdatedAt.IsZero() {
		entry["modtime"] = auth.UpdatedAt
		entry["updated_at"] = auth.UpdatedAt
	}
	if !auth.LastRefreshedAt.IsZero() {
		entry["last_refresh"] = auth.LastRefreshedAt
	}
	if !auth.NextRetryAfter.IsZero() {
		entry["next_retry_after"] = auth.NextRetryAfter
	}
	if claims := extractCodexIDTokenClaims(auth); claims != nil {
		entry["id_token"] = claims
	}
	if auth.Metadata != nil {
		if usage, ok := auth.Metadata["usage"]; ok && usage != nil {
			entry["usage"] = usage
		}
	}
	// Expose priority from Attributes (set by synthesizer from JSON "priority" field).
	// Fall back to Metadata for auths registered via UploadAuthFile (no synthesizer).
	if p := strings.TrimSpace(authAttribute(auth, "priority")); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			entry["priority"] = parsed
		}
	} else if auth.Metadata != nil {
		if rawPriority, ok := auth.Metadata["priority"]; ok {
			switch v := rawPriority.(type) {
			case float64:
				entry["priority"] = int(v)
			case int:
				entry["priority"] = v
			case string:
				if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
					entry["priority"] = parsed
				}
			}
		}
	}
	// Expose note from Attributes (set by synthesizer from JSON "note" field).
	// Fall back to Metadata for auths registered via UploadAuthFile (no synthesizer).
	if note := strings.TrimSpace(authAttribute(auth, "note")); note != "" {
		entry["note"] = note
	} else if auth.Metadata != nil {
		if rawNote, ok := auth.Metadata["note"].(string); ok {
			if trimmed := strings.TrimSpace(rawNote); trimmed != "" {
				entry["note"] = trimmed
			}
		}
	}
	return entry
}

func extractCodexIDTokenClaims(auth *coreauth.Auth) gin.H {
	if auth == nil || auth.Metadata == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") {
		return nil
	}
	idTokenRaw, ok := auth.Metadata["id_token"].(string)
	if !ok {
		return nil
	}
	idToken := strings.TrimSpace(idTokenRaw)
	if idToken == "" {
		return nil
	}
	claims, err := codex.ParseJWTToken(idToken)
	if err != nil || claims == nil {
		return nil
	}

	result := gin.H{}
	if v := strings.TrimSpace(claims.CodexAuthInfo.ChatgptAccountID); v != "" {
		result["chatgpt_account_id"] = v
	}
	if v := strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType); v != "" {
		result["plan_type"] = v
	}
	if v := claims.CodexAuthInfo.ChatgptSubscriptionActiveStart; v != nil {
		result["chatgpt_subscription_active_start"] = v
	}
	if v := claims.CodexAuthInfo.ChatgptSubscriptionActiveUntil; v != nil {
		result["chatgpt_subscription_active_until"] = v
	}
	if result["chatgpt_subscription_active_start"] != nil || result["chatgpt_subscription_active_until"] != nil {
		result["subscription"] = "active"
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// Download single auth file by name
func (h *Handler) DownloadAuthFile(c *gin.Context) {
	name := c.Query("name")
	if name == "" || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		c.JSON(400, gin.H{"error": "invalid name"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		c.JSON(400, gin.H{"error": "name must end with .json"})
		return
	}
	if h.authStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store unavailable"})
		return
	}
	data, err := h.authStore.ReadByName(c.Request.Context(), name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(404, gin.H{"error": "file not found"})
		} else {
			c.JSON(500, gin.H{"error": fmt.Sprintf("failed to read auth file: %v", err)})
		}
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
	c.Data(200, "application/json", data)
}

func (h *Handler) findManagedAuth(name string) *coreauth.Auth {
	if h == nil || h.authManager == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if auth, ok := h.authManager.GetByID(name); ok {
		if isManagedStoredAuth(auth) {
			return auth
		}
		return nil
	}
	auths := h.authManager.List()
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		if !isManagedStoredAuth(auth) {
			continue
		}
		if strings.TrimSpace(auth.FileName) == name {
			return auth
		}
	}
	return nil
}
