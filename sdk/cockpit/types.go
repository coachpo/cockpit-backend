// Package cockpit provides the core service implementation for Cockpit.
// and integration with various AI service providers through a unified interface.
package cockpit

import (
	"context"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"github.com/coachpo/cockpit-backend/internal/watcher"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

type TokenClientProvider interface {
	// Load loads token-based clients from the configured source.
	//
	// Parameters:
	//   - ctx: The context for the loading operation
	//   - cfg: The application configuration
	//
	// Returns:
	//   - *TokenClientResult: The result containing loaded clients
	//   - error: An error if loading fails
	Load(ctx context.Context, cfg *config.Config) (*TokenClientResult, error)
}

type TokenClientResult struct {
	// SuccessfulAuthed is the number of successfully authenticated clients.
	SuccessfulAuthed int
}

// APIKeyClientProvider loads clients backed directly by configured API keys.
// It provides an interface for loading API key-based clients for various AI service providers.
type APIKeyClientProvider interface {
	// Load loads API key-based clients from the configuration.
	//
	// Parameters:
	//   - ctx: The context for the loading operation
	//   - cfg: The application configuration
	//
	// Returns:
	//   - *APIKeyClientResult: The result containing loaded clients
	//   - error: An error if loading fails
	Load(ctx context.Context, cfg *config.Config) (*APIKeyClientResult, error)
}

// APIKeyClientResult is returned by APIKeyClientProvider.Load()
type APIKeyClientResult struct {
	// CodexKeyCount is the number of Codex API keys loaded
	CodexKeyCount int
}

// The reload callback receives the updated configuration when changes are detected.
type WatcherFactory func(reload func(*config.Config), configSource nacos.ConfigSource, authStore nacos.WatchableAuthStore) (*WatcherWrapper, error)

// WatcherWrapper exposes the subset of watcher methods required by the SDK.
type WatcherWrapper struct {
	start func(ctx context.Context) error
	stop  func() error

	setConfig             func(cfg *config.Config)
	snapshotAuths         func() []*coreauth.Auth
	setUpdateQueue        func(queue chan<- watcher.AuthUpdate)
	dispatchRuntimeUpdate func(update watcher.AuthUpdate) bool
}

// Start proxies to the underlying watcher Start implementation.
func (w *WatcherWrapper) Start(ctx context.Context) error {
	if w == nil || w.start == nil {
		return nil
	}
	return w.start(ctx)
}

// Stop proxies to the underlying watcher Stop implementation.
func (w *WatcherWrapper) Stop() error {
	if w == nil || w.stop == nil {
		return nil
	}
	return w.stop()
}

// SetConfig updates the watcher configuration cache.
func (w *WatcherWrapper) SetConfig(cfg *config.Config) {
	if w == nil || w.setConfig == nil {
		return
	}
	w.setConfig(cfg)
}

// DispatchRuntimeAuthUpdate forwards runtime auth updates (e.g., websocket providers)
// into the watcher-managed auth update queue when available.
// Returns true if the update was enqueued successfully.
func (w *WatcherWrapper) DispatchRuntimeAuthUpdate(update watcher.AuthUpdate) bool {
	if w == nil || w.dispatchRuntimeUpdate == nil {
		return false
	}
	return w.dispatchRuntimeUpdate(update)
}

func (w *WatcherWrapper) SnapshotAuths() []*coreauth.Auth {
	if w == nil || w.snapshotAuths == nil {
		return nil
	}
	return w.snapshotAuths()
}

// SetAuthUpdateQueue registers the channel used to propagate auth updates.
func (w *WatcherWrapper) SetAuthUpdateQueue(queue chan<- watcher.AuthUpdate) {
	if w == nil || w.setUpdateQueue == nil {
		return
	}
	w.setUpdateQueue(queue)
}
