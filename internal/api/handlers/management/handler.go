// Package management provides the management API handlers and middleware
// for configuring the server and managing auth files.
package management

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Handler aggregates config reference, persistence path and helpers.
type Handler struct {
	cfg             *config.Config
	persistedConfig *config.Config
	configFilePath  string
	configSaver     func(*config.Config) error
	mu              sync.Mutex
	authManager     *coreauth.Manager
	authStore       nacos.WatchableAuthStore
	logDir          string
	postAuthHook    coreauth.PostAuthHook
}

// NewHandler creates a new management handler instance.
func NewHandler(cfg *config.Config, configFilePath string, manager *coreauth.Manager, store nacos.WatchableAuthStore) *Handler {
	persistedConfig, _ := cloneConfig(cfg)

	return &Handler{
		cfg:             cfg,
		persistedConfig: persistedConfig,
		configFilePath:  configFilePath,
		authManager:     manager,
		authStore:       store,
	}
}

func (h *Handler) SetConfigSaver(saver func(*config.Config) error) {
	h.configSaver = saver
}

// NewHandlerWithoutConfigFilePath creates a management handler without persistence wiring.
func NewHandlerWithoutConfigFilePath(cfg *config.Config, manager *coreauth.Manager) *Handler {
	return NewHandler(cfg, "", manager, nil)
}

// SetConfig updates the in-memory config reference when the server hot-reloads.
func (h *Handler) SetConfig(cfg *config.Config) {
	h.cfg = cfg
	h.persistedConfig, _ = cloneConfig(cfg)
}

// SetAuthManager updates the auth manager reference used by management endpoints.
func (h *Handler) SetAuthManager(manager *coreauth.Manager) { h.authManager = manager }

func (h *Handler) SetAuthStore(store nacos.WatchableAuthStore) { h.authStore = store }

// SetLogDirectory updates the directory where main.log should be looked up.
func (h *Handler) SetLogDirectory(dir string) {
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	h.logDir = dir
}

// SetPostAuthHook registers a hook to be called after auth record creation but before persistence.
func (h *Handler) SetPostAuthHook(hook coreauth.PostAuthHook) {
	h.postAuthHook = hook
}

// Middleware enforces Bearer-only access control for management endpoints.
func (h *Handler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := ""
		if h.cfg != nil {
			secret = strings.TrimSpace(h.cfg.RemoteManagement.SecretKey)
		}
		if secret == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "remote management key not set"})
			return
		}

		var provided string
		if ah := strings.TrimSpace(c.GetHeader("Authorization")); ah != "" {
			parts := strings.SplitN(ah, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				provided = strings.TrimSpace(parts[1])
			}
		}

		if provided == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing management key"})
			return
		}

		if !managementSecretMatches(secret, provided) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid management key"})
			return
		}

		c.Next()
	}
}

func managementSecretMatches(secret string, provided string) bool {
	if secret == "" || provided == "" {
		return false
	}
	if strings.HasPrefix(secret, "$2a$") || strings.HasPrefix(secret, "$2b$") || strings.HasPrefix(secret, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(secret), []byte(provided)) == nil
	}
	return subtle.ConstantTimeCompare([]byte(secret), []byte(provided)) == 1
}

func cloneConfig(cfg *config.Config) (*config.Config, error) {
	if cfg == nil {
		return &config.Config{}, nil
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	clone := &config.Config{}
	if err = yaml.Unmarshal(raw, clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func restoreConfigSnapshot(dst **config.Config, snapshot *config.Config) {
	if snapshot == nil {
		return
	}
	if *dst == nil {
		*dst = snapshot
		return
	}
	**dst = *snapshot
}

func (h *Handler) persist(c *gin.Context) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	saver := h.configSaver
	if saver == nil {
		snapshot, errClone := cloneConfig(h.persistedConfig)
		if errClone == nil {
			restoreConfigSnapshot(&h.cfg, snapshot)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save config: %v", nacos.ErrStaticMode)})
		return false
	}
	if err := saver(h.cfg); err != nil {
		snapshot, errClone := cloneConfig(h.persistedConfig)
		if errClone == nil {
			restoreConfigSnapshot(&h.cfg, snapshot)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save config: %v", err)})
		return false
	}
	h.persistedConfig, _ = cloneConfig(h.cfg)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
	return true
}

// Helper methods for simple types
func (h *Handler) updateBoolField(c *gin.Context, set func(bool)) {
	var body struct {
		Value *bool `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	set(*body.Value)
	h.persist(c)
}

func (h *Handler) updateIntField(c *gin.Context, set func(int)) {
	var body struct {
		Value *int `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	set(*body.Value)
	h.persist(c)
}

func (h *Handler) updateStringField(c *gin.Context, set func(string)) {
	var body struct {
		Value *string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	set(*body.Value)
	h.persist(c)
}
