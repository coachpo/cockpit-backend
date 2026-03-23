package management

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/gin-gonic/gin"
)

type apiKeysEnvelope struct {
	Items []string `json:"items"`
}

type apiKeysRequest struct {
	Items *[]string `json:"items"`
}

func (h *Handler) patchStringList(c *gin.Context, target *[]string, after func()) {
	var body struct {
		Old   *string `json:"old"`
		New   *string `json:"new"`
		Index *int    `json:"index"`
		Value *string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	if body.Index != nil && body.Value != nil && *body.Index >= 0 && *body.Index < len(*target) {
		(*target)[*body.Index] = *body.Value
		if after != nil {
			after()
		}
		h.persist(c)
		return
	}
	if body.Old != nil && body.New != nil {
		for i := range *target {
			if (*target)[i] == *body.Old {
				(*target)[i] = *body.New
				if after != nil {
					after()
				}
				h.persist(c)
				return
			}
		}
		*target = append(*target, *body.New)
		if after != nil {
			after()
		}
		h.persist(c)
		return
	}
	c.JSON(400, gin.H{"error": "missing fields"})
}

func (h *Handler) deleteFromStringList(c *gin.Context, target *[]string, after func()) {
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(*target) {
			*target = append((*target)[:idx], (*target)[idx+1:]...)
			if after != nil {
				after()
			}
			h.persist(c)
			return
		}
	}
	if val := strings.TrimSpace(c.Query("value")); val != "" {
		out := make([]string, 0, len(*target))
		for _, v := range *target {
			if strings.TrimSpace(v) != val {
				out = append(out, v)
			}
		}
		*target = out
		if after != nil {
			after()
		}
		h.persist(c)
		return
	}
	c.JSON(400, gin.H{"error": "missing index or value"})
}

// api-keys
func (h *Handler) GetAPIKeys(c *gin.Context) {
	items := append([]string(nil), h.cfg.APIKeys...)
	c.JSON(200, apiKeysEnvelope{Items: items})
}

func (h *Handler) PutAPIKeys(c *gin.Context) {
	var body apiKeysRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.Items == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	h.cfg.APIKeys = append([]string(nil), (*body.Items)...)
	h.persist(c)
}

func (h *Handler) PatchAPIKeys(c *gin.Context) {
	h.patchStringList(c, &h.cfg.APIKeys, func() {})
}
func (h *Handler) DeleteAPIKeys(c *gin.Context) {
	h.deleteFromStringList(c, &h.cfg.APIKeys, func() {})
}

// codex-api-key: []CodexKey
func (h *Handler) GetCodexKeys(c *gin.Context) {
	c.JSON(200, gin.H{"codex-api-key": h.cfg.CodexKey})
}
func (h *Handler) PutCodexKeys(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.CodexKey
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.CodexKey `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	normalized := make([]config.CodexKey, 0, len(arr))
	for i := range arr {
		entry := arr[i]
		config.NormalizeCodexKey(&entry)
		if entry.BaseURL == "" {
			c.JSON(400, gin.H{"error": "base-url is required"})
			return
		}
		normalized = append(normalized, entry)
	}
	h.cfg.CodexKey = normalized
	h.cfg.SanitizeCodexKeys()
	h.persist(c)
}
func (h *Handler) PatchCodexKey(c *gin.Context) {
	type codexKeyPatch struct {
		APIKey     *string            `json:"api-key"`
		BaseURL    *string            `json:"base-url"`
		Priority   *int               `json:"priority"`
		Websockets *bool              `json:"websockets"`
		Headers    *map[string]string `json:"headers"`
	}
	var body struct {
		Index *int           `json:"index"`
		Match *string        `json:"match"`
		Value *codexKeyPatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.CodexKey) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Match != nil {
		match := config.NormalizeCodexAPIKey(*body.Match)
		for i := range h.cfg.CodexKey {
			if config.NormalizeCodexAPIKey(h.cfg.CodexKey[i].APIKey) == match {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.CodexKey[targetIndex]
	config.NormalizeCodexKey(&entry)
	if body.Value.APIKey != nil {
		entry.APIKey = *body.Value.APIKey
	}
	if body.Value.BaseURL != nil {
		if strings.TrimSpace(*body.Value.BaseURL) == "" {
			c.JSON(400, gin.H{"error": "base-url is required"})
			return
		}
		entry.BaseURL = *body.Value.BaseURL
	}
	if body.Value.Priority != nil {
		entry.Priority = *body.Value.Priority
	}
	if body.Value.Websockets != nil {
		entry.Websockets = *body.Value.Websockets
	}
	if body.Value.Headers != nil {
		entry.Headers = *body.Value.Headers
	}
	config.NormalizeCodexKey(&entry)
	h.cfg.CodexKey[targetIndex] = entry
	h.cfg.SanitizeCodexKeys()
	h.persist(c)
}

func (h *Handler) DeleteCodexKey(c *gin.Context) {
	if val := config.NormalizeCodexAPIKey(c.Query("api-key")); val != "" {
		out := make([]config.CodexKey, 0, len(h.cfg.CodexKey))
		for _, v := range h.cfg.CodexKey {
			if config.NormalizeCodexAPIKey(v.APIKey) != val {
				out = append(out, v)
			}
		}
		h.cfg.CodexKey = out
		h.cfg.SanitizeCodexKeys()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(h.cfg.CodexKey) {
			h.cfg.CodexKey = append(h.cfg.CodexKey[:idx], h.cfg.CodexKey[idx+1:]...)
			h.cfg.SanitizeCodexKeys()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing api-key or index"})
}
