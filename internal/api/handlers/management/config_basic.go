package management

import (
	"net/http"
	"strings"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/gin-gonic/gin"
)

type runtimeSettingsResponse struct {
	WebsocketAuth    bool   `json:"ws-auth"`
	RequestRetry     int    `json:"request-retry"`
	MaxRetryInterval int    `json:"max-retry-interval"`
	RoutingStrategy  string `json:"routing-strategy"`
	SwitchProject    bool   `json:"switch-project"`
}

type runtimeSettingsRequest struct {
	WebsocketAuth    *bool   `json:"ws-auth"`
	RequestRetry     *int    `json:"request-retry"`
	MaxRetryInterval *int    `json:"max-retry-interval"`
	RoutingStrategy  *string `json:"routing-strategy"`
	SwitchProject    *bool   `json:"switch-project"`
}

func (h *Handler) currentRuntimeSettings() runtimeSettingsResponse {
	strategy, ok := normalizeRoutingStrategy(h.cfg.Routing.Strategy)
	if !ok {
		strategy = strings.TrimSpace(h.cfg.Routing.Strategy)
	}

	return runtimeSettingsResponse{
		WebsocketAuth:    h.cfg.WebsocketAuth,
		RequestRetry:     h.cfg.RequestRetry,
		MaxRetryInterval: h.cfg.MaxRetryInterval,
		RoutingStrategy:  strategy,
		SwitchProject:    h.cfg.QuotaExceeded.SwitchProject,
	}
}

func (h *Handler) GetRuntimeSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.currentRuntimeSettings())
}

func (h *Handler) PutRuntimeSettings(c *gin.Context) {
	var body runtimeSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil ||
		body.WebsocketAuth == nil ||
		body.RequestRetry == nil ||
		body.MaxRetryInterval == nil ||
		body.RoutingStrategy == nil ||
		body.SwitchProject == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	normalizedStrategy, ok := normalizeRoutingStrategy(*body.RoutingStrategy)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid strategy"})
		return
	}

	h.cfg.WebsocketAuth = *body.WebsocketAuth
	h.cfg.RequestRetry = *body.RequestRetry
	h.cfg.MaxRetryInterval = *body.MaxRetryInterval
	h.cfg.Routing.Strategy = normalizedStrategy
	h.cfg.QuotaExceeded.SwitchProject = *body.SwitchProject
	h.persist(c)
}

// Websocket auth
func (h *Handler) GetWebsocketAuth(c *gin.Context) {
	c.JSON(200, gin.H{"ws-auth": h.cfg.WebsocketAuth})
}
func (h *Handler) PutWebsocketAuth(c *gin.Context) {
	h.updateBoolField(c, func(v bool) { h.cfg.WebsocketAuth = v })
}

// Request retry
func (h *Handler) GetRequestRetry(c *gin.Context) {
	c.JSON(200, gin.H{"request-retry": h.cfg.RequestRetry})
}
func (h *Handler) PutRequestRetry(c *gin.Context) {
	h.updateIntField(c, func(v int) { h.cfg.RequestRetry = v })
}

// Max retry interval
func (h *Handler) GetMaxRetryInterval(c *gin.Context) {
	c.JSON(200, gin.H{"max-retry-interval": h.cfg.MaxRetryInterval})
}
func (h *Handler) PutMaxRetryInterval(c *gin.Context) {
	h.updateIntField(c, func(v int) { h.cfg.MaxRetryInterval = v })
}

func normalizeRoutingStrategy(strategy string) (string, bool) {
	return config.NormalizeRoutingStrategy(strategy)
}

// RoutingStrategy
func (h *Handler) GetRoutingStrategy(c *gin.Context) {
	strategy, ok := normalizeRoutingStrategy(h.cfg.Routing.Strategy)
	if !ok {
		c.JSON(200, gin.H{"strategy": strings.TrimSpace(h.cfg.Routing.Strategy)})
		return
	}
	c.JSON(200, gin.H{"strategy": strategy})
}
func (h *Handler) PutRoutingStrategy(c *gin.Context) {
	var body struct {
		Value *string `json:"value"`
	}
	if errBindJSON := c.ShouldBindJSON(&body); errBindJSON != nil || body.Value == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	normalized, ok := normalizeRoutingStrategy(*body.Value)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid strategy"})
		return
	}
	h.cfg.Routing.Strategy = normalized
	h.persist(c)
}
