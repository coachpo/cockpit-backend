// Package api provides the HTTP API server implementation for Cockpit.
// The server supports hot-reloading of clients and configuration.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	managementHandlers "github.com/coachpo/cockpit-backend/internal/api/handlers/management"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/logging"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	sdkaccess "github.com/coachpo/cockpit-backend/sdk/access"
	"github.com/coachpo/cockpit-backend/sdk/api/handlers"
	"github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const oauthCallbackSuccessHTML = `<html><head><meta charset="utf-8"><title>Authentication successful</title><script>setTimeout(function(){window.close();},5000);</script></head><body><h1>Authentication successful!</h1><p>You can close this window.</p><p>This window will close automatically in 5 seconds.</p></body></html>`

const managementPasswordEnvKey = "MANAGEMENT_PASSWORD"

// Server represents the main API server.
// It encapsulates the Gin engine, HTTP server, handlers, and configuration.
type Server struct {
	// engine is the Gin web framework engine instance.
	engine *gin.Engine

	// server is the underlying HTTP server.
	server *http.Server

	// handlers contains the API handlers for processing requests.
	handlers *handlers.BaseAPIHandler

	// cfg holds the current server configuration.
	cfg *config.Config

	// oldConfigYaml stores a YAML snapshot of the previous configuration for change detection.
	// This prevents issues when the config object is modified in place by Management API.
	oldConfigYaml []byte

	// accessManager handles request authentication providers.
	accessManager *sdkaccess.Manager

	// currentPath is the absolute path to the current working directory.
	currentPath string

	// wsRoutes tracks registered websocket upgrade paths.
	wsRouteMu     sync.Mutex
	wsRoutes      map[string]struct{}
	wsAuthChanged func(bool, bool)
	wsAuthEnabled atomic.Bool

	// management handler
	mgmt      *managementHandlers.Handler
	authStore nacos.WatchableAuthStore

	// managementRoutesRegistered tracks whether the management routes have been attached to the engine.
	managementRoutesRegistered atomic.Bool

	keepAliveEnabled   bool
	keepAliveTimeout   time.Duration
	keepAliveOnTimeout func()
	keepAliveHeartbeat chan struct{}
	keepAliveStop      chan struct{}
}

// NewServer creates and initializes a new API server instance.
// It sets up the Gin engine, middleware, routes, and handlers.
//
// Parameters:
//   - cfg: The server configuration
//   - authManager: core runtime auth manager
//   - accessManager: request authentication manager
//
// Returns:
//   - *Server: A new server instance
func NewServer(cfg *config.Config, authManager *auth.Manager, accessManager *sdkaccess.Manager, authStore nacos.WatchableAuthStore, opts ...ServerOption) *Server {
	optionState := &serverOptionConfig{}
	for i := range opts {
		opts[i](optionState)
	}
	// Set gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create gin engine
	engine := gin.New()
	if optionState.engineConfigurator != nil {
		optionState.engineConfigurator(engine)
	}

	// Add middleware
	engine.Use(logging.GinLogrusLogger())
	engine.Use(logging.GinLogrusRecovery())
	for _, mw := range optionState.extraMiddleware {
		engine.Use(mw)
	}

	engine.Use(corsMiddleware())
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}

	// Create server instance
	s := &Server{
		engine:        engine,
		handlers:      handlers.NewBaseAPIHandlers(&cfg.SDKConfig, authManager),
		cfg:           cfg,
		accessManager: accessManager,
		currentPath:   wd,
		wsRoutes:      make(map[string]struct{}),
		authStore:     authStore,
	}
	s.wsAuthEnabled.Store(cfg.WebsocketAuth)
	// Save initial YAML snapshot
	s.oldConfigYaml, _ = yaml.Marshal(cfg)
	s.applyAccessConfig(nil, cfg)
	if authManager != nil {
		authManager.SetRetryConfig(cfg.RequestRetry, time.Duration(cfg.MaxRetryInterval)*time.Second, cfg.MaxRetryCredentials)
	}
	auth.SetQuotaCooldownDisabled(cfg.DisableCooling)
	// Initialize management handler
	s.mgmt = managementHandlers.NewHandler(cfg, authManager, authStore)
	if optionState.configSaver != nil {
		s.mgmt.SetConfigSaver(optionState.configSaver)
	}
	logDir := logging.ResolveLogDirectory(cfg)
	s.mgmt.SetLogDirectory(logDir)
	if optionState.postAuthHook != nil {
		s.mgmt.SetPostAuthHook(optionState.postAuthHook)
	}

	// Setup routes
	s.setupRoutes()

	// Apply additional router configurators from options
	if optionState.routerConfigurator != nil {
		optionState.routerConfigurator(engine, s.handlers, cfg)
	}

	s.registerManagementRoutes()

	if optionState.keepAliveEnabled {
		s.enableKeepAlive(optionState.keepAliveTimeout, optionState.keepAliveOnTimeout)
	}

	// Create HTTP server
	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler: engine,
	}

	return s
}

// AttachWebsocketRoute registers a websocket upgrade handler on the primary Gin engine.
// The handler is served as-is without additional middleware beyond the standard stack already configured.
func (s *Server) AttachWebsocketRoute(path string, handler http.Handler) {
	if s == nil || s.engine == nil || handler == nil {
		return
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		trimmed = "/v1/ws"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	s.wsRouteMu.Lock()
	if _, exists := s.wsRoutes[trimmed]; exists {
		s.wsRouteMu.Unlock()
		return
	}
	s.wsRoutes[trimmed] = struct{}{}
	s.wsRouteMu.Unlock()

	authMiddleware := AuthMiddleware(s.accessManager)
	conditionalAuth := func(c *gin.Context) {
		if !s.wsAuthEnabled.Load() {
			c.Next()
			return
		}
		authMiddleware(c)
	}
	finalHandler := func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}

	s.engine.GET(trimmed, conditionalAuth, finalHandler)
}

// Start begins listening for and serving HTTP requests.
// It's a blocking call and will only return on an unrecoverable error.
//
// Returns:
//   - error: An error if the server fails to start
func (s *Server) Start() error {
	if s == nil || s.server == nil {
		return fmt.Errorf("failed to start HTTP server: server not initialized")
	}

	log.Debugf("Starting API server on %s", s.server.Addr)
	if errServe := s.server.ListenAndServe(); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
		return fmt.Errorf("failed to start HTTP server: %v", errServe)
	}

	return nil
}

// Stop gracefully shuts down the API server without interrupting any
// active connections.
//
// Parameters:
//   - ctx: The context for graceful shutdown
//
// Returns:
//   - error: An error if the server fails to stop
func (s *Server) Stop(ctx context.Context) error {
	log.Debug("Stopping API server...")

	if s.keepAliveEnabled {
		select {
		case s.keepAliveStop <- struct{}{}:
		default:
		}
	}

	// Shutdown the HTTP server.
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %v", err)
	}

	log.Debug("API server stopped")
	return nil
}

// corsMiddleware returns a Gin middleware handler that adds CORS headers
// to every response, allowing cross-origin requests.
//
// Returns:
//   - gin.HandlerFunc: The CORS middleware handler
func corsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Length",
			"Content-Type",
			"Authorization",
			"Accept",
			"Accept-Encoding",
			"X-Requested-With",
		},
	})
}

// (management handlers moved to internal/api/handlers/management)

func AuthMiddleware(manager *sdkaccess.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.Next()
			return
		}

		result, err := manager.Authenticate(c.Request.Context(), c.Request)
		if err == nil {
			if result != nil {
				c.Set("apiKey", result.Principal)
				c.Set("accessProvider", result.Provider)
				if len(result.Metadata) > 0 {
					c.Set("accessMetadata", result.Metadata)
				}
			}
			c.Next()
			return
		}

		statusCode := err.HTTPStatusCode()
		if statusCode >= http.StatusInternalServerError {
			log.Errorf("authentication middleware error: %v", err)
		}
		c.AbortWithStatusJSON(statusCode, gin.H{"error": err.Message})
	}
}

func managementPassword() string {
	value, ok := os.LookupEnv(managementPasswordEnvKey)
	if !ok {
		return ""
	}

	return strings.TrimSpace(value)
}

func ManagementAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		expectedPassword := managementPassword()
		if expectedPassword == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Management password is not configured"})
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		scheme, token, ok := strings.Cut(authHeader, " ")
		token = strings.TrimSpace(token)
		if !ok || !strings.EqualFold(strings.TrimSpace(scheme), "Bearer") || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing management bearer token"})
			return
		}

		if token != expectedPassword {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid management bearer token"})
			return
		}

		c.Next()
	}
}
