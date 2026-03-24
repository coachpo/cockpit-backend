// Package cockpit provides the core service implementation for Cockpit.
// and integration with various AI service providers through a unified interface.
package cockpit

import (
	"fmt"

	configaccess "github.com/coachpo/cockpit-backend/internal/access/config_access"
	"github.com/coachpo/cockpit-backend/internal/api"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	sdkaccess "github.com/coachpo/cockpit-backend/sdk/access"
	sdkAuth "github.com/coachpo/cockpit-backend/sdk/auth"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

// Builder constructs a Service instance with customizable providers.
// It provides a fluent interface for configuring all aspects of the service
type Builder struct {
	// cfg holds the application configuration.
	cfg *config.Config

	// tokenProvider handles loading token-based clients.
	tokenProvider TokenClientProvider

	// apiKeyProvider handles loading API key-based clients.
	apiKeyProvider APIKeyClientProvider

	watcherFactory WatcherFactory
	configSource   nacos.ConfigSource
	authStore      nacos.WatchableAuthStore

	// hooks provides lifecycle callbacks.
	hooks Hooks

	authManager *sdkAuth.Manager

	// accessManager handles request authentication providers.
	accessManager *sdkaccess.Manager

	// coreManager handles core authentication and execution.
	coreManager *coreauth.Manager

	// serverOptions contains additional server configuration options.
	serverOptions []api.ServerOption
}

// Hooks allows callers to plug into service lifecycle stages.
// These callbacks provide opportunities to perform custom initialization
// and cleanup operations during service startup and shutdown.
type Hooks struct {
	// OnBeforeStart is called before the service starts, allowing configuration
	// modifications or additional setup.
	OnBeforeStart func(*config.Config)

	// OnAfterStart is called after the service has started successfully,
	// providing access to the service instance for additional operations.
	OnAfterStart func(*Service)
}

// NewBuilder creates a Builder with default dependencies left unset.
// Use the fluent interface methods to configure the service before calling Build().
//
// Returns:
//   - *Builder: A new builder instance ready for configuration
func NewBuilder() *Builder {
	return &Builder{}
}

// WithConfig sets the configuration instance used by the service.
//
// Parameters:
//   - cfg: The application configuration
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithConfig(cfg *config.Config) *Builder {
	b.cfg = cfg
	return b
}

// WithTokenClientProvider overrides the provider responsible for token-backed clients.
func (b *Builder) WithTokenClientProvider(provider TokenClientProvider) *Builder {
	b.tokenProvider = provider
	return b
}

// WithAPIKeyClientProvider overrides the provider responsible for API key-backed clients.
func (b *Builder) WithAPIKeyClientProvider(provider APIKeyClientProvider) *Builder {
	b.apiKeyProvider = provider
	return b
}

// WithWatcherFactory allows customizing the watcher factory that handles reloads.
func (b *Builder) WithWatcherFactory(factory WatcherFactory) *Builder {
	b.watcherFactory = factory
	return b
}

// WithConfigSource overrides the configuration source used by the service.
func (b *Builder) WithConfigSource(source nacos.ConfigSource) *Builder {
	b.configSource = source
	return b
}

// WithAuthStore overrides the auth store used by the service.
func (b *Builder) WithAuthStore(store nacos.WatchableAuthStore) *Builder {
	b.authStore = store
	return b
}

// WithHooks registers lifecycle hooks executed around service startup.
func (b *Builder) WithHooks(h Hooks) *Builder {
	b.hooks = h
	return b
}

// WithAuthManager overrides the authentication manager used for token lifecycle operations.
func (b *Builder) WithAuthManager(mgr *sdkAuth.Manager) *Builder {
	b.authManager = mgr
	return b
}

// WithRequestAccessManager overrides the request authentication manager.
func (b *Builder) WithRequestAccessManager(mgr *sdkaccess.Manager) *Builder {
	b.accessManager = mgr
	return b
}

// WithCoreAuthManager overrides the runtime auth manager responsible for request execution.
func (b *Builder) WithCoreAuthManager(mgr *coreauth.Manager) *Builder {
	b.coreManager = mgr
	return b
}

// WithServerOptions appends server configuration options used during construction.
func (b *Builder) WithServerOptions(opts ...api.ServerOption) *Builder {
	b.serverOptions = append(b.serverOptions, opts...)
	return b
}

// WithPostAuthHook registers a hook to be called after an Auth record is created
// but before it is persisted to storage.
func (b *Builder) WithPostAuthHook(hook coreauth.PostAuthHook) *Builder {
	if hook == nil {
		return b
	}
	b.serverOptions = append(b.serverOptions, api.WithPostAuthHook(hook))
	return b
}

// Build validates inputs, applies defaults, and returns a ready-to-run service.
func (b *Builder) Build() (*Service, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("cockpit: configuration is required")
	}
	tokenProvider := b.tokenProvider
	if tokenProvider == nil {
		tokenProvider = NewTokenClientProvider()
	}

	apiKeyProvider := b.apiKeyProvider
	if apiKeyProvider == nil {
		apiKeyProvider = NewAPIKeyClientProvider()
	}

	watcherFactory := b.watcherFactory
	if watcherFactory == nil {
		watcherFactory = defaultWatcherFactory
	}

	configSource := b.configSource
	if configSource == nil {
		return nil, fmt.Errorf("cockpit: config source is required")
	}

	authStore := b.authStore
	if authStore == nil {
		return nil, fmt.Errorf("cockpit: auth store is required")
	}

	authManager := b.authManager
	if authManager == nil {
		authManager = newDefaultAuthManager(authStore)
	}

	accessManager := b.accessManager
	if accessManager == nil {
		accessManager = sdkaccess.NewManager()
	}

	configaccess.Register(&b.cfg.SDKConfig)
	accessManager.SetProviders(sdkaccess.RegisteredProviders())

	coreManager := b.coreManager
	if coreManager == nil {
		normalizedStrategy := "round-robin"
		if b.cfg != nil {
			var ok bool
			normalizedStrategy, ok = config.NormalizeRoutingStrategy(b.cfg.Routing.Strategy)
			if !ok {
				return nil, fmt.Errorf("cockpit: invalid routing strategy %q", b.cfg.Routing.Strategy)
			}
		}
		var selector coreauth.Selector
		switch normalizedStrategy {
		case "fill-first":
			selector = &coreauth.FillFirstSelector{}
		default:
			selector = &coreauth.RoundRobinSelector{}
		}

		coreManager = coreauth.NewManager(authStore, selector, nil)
	}
	// Attach a default RoundTripper provider so providers can opt-in per-auth transports.
	coreManager.SetRoundTripperProvider(newDefaultRoundTripperProvider())
	coreManager.SetConfig(b.cfg)

	service := &Service{
		cfg:            b.cfg,
		configSource:   configSource,
		authStore:      authStore,
		tokenProvider:  tokenProvider,
		apiKeyProvider: apiKeyProvider,
		watcherFactory: watcherFactory,
		hooks:          b.hooks,
		authManager:    authManager,
		accessManager:  accessManager,
		coreManager:    coreManager,
		serverOptions:  append([]api.ServerOption(nil), b.serverOptions...),
	}
	return service, nil
}
