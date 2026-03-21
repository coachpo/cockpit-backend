package api

import (
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/sdk/api/handlers"
	"github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

type serverOptionConfig struct {
	extraMiddleware    []gin.HandlerFunc
	engineConfigurator func(*gin.Engine)
	routerConfigurator func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config)
	localPassword      string
	configSaver        func(*config.Config) error
	keepAliveEnabled   bool
	keepAliveTimeout   time.Duration
	keepAliveOnTimeout func()
	postAuthHook       auth.PostAuthHook
}

// ServerOption customises HTTP server construction.
type ServerOption func(*serverOptionConfig)

// WithMiddleware appends additional Gin middleware during server construction.
func WithMiddleware(mw ...gin.HandlerFunc) ServerOption {
	return func(cfg *serverOptionConfig) {
		cfg.extraMiddleware = append(cfg.extraMiddleware, mw...)
	}
}

// WithEngineConfigurator allows callers to mutate the Gin engine prior to middleware setup.
func WithEngineConfigurator(fn func(*gin.Engine)) ServerOption {
	return func(cfg *serverOptionConfig) {
		cfg.engineConfigurator = fn
	}
}

// WithRouterConfigurator appends a callback after default routes are registered.
func WithRouterConfigurator(fn func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config)) ServerOption {
	return func(cfg *serverOptionConfig) {
		cfg.routerConfigurator = fn
	}
}

// WithLocalManagementPassword stores a runtime-only management password accepted for localhost requests.
func WithLocalManagementPassword(password string) ServerOption {
	return func(cfg *serverOptionConfig) {
		cfg.localPassword = password
	}
}

func WithConfigSaver(saver func(*config.Config) error) ServerOption {
	return func(cfg *serverOptionConfig) {
		cfg.configSaver = saver
	}
}

// WithKeepAliveEndpoint enables a keep-alive endpoint with the provided timeout and callback.
func WithKeepAliveEndpoint(timeout time.Duration, onTimeout func()) ServerOption {
	return func(cfg *serverOptionConfig) {
		if timeout <= 0 || onTimeout == nil {
			return
		}
		cfg.keepAliveEnabled = true
		cfg.keepAliveTimeout = timeout
		cfg.keepAliveOnTimeout = onTimeout
	}
}

// WithPostAuthHook registers a hook to be called after auth record creation.
func WithPostAuthHook(hook auth.PostAuthHook) ServerOption {
	return func(cfg *serverOptionConfig) {
		cfg.postAuthHook = hook
	}
}
