// Package watcher watches config/auth changes and triggers hot reloads.
// It supports both Nacos-backed and static file-backed config sources.
package watcher

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"gopkg.in/yaml.v3"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

type Watcher struct {
	configPath        string
	authDir           string
	config            *config.Config
	clientsMutex      sync.RWMutex
	configReloadMu    sync.Mutex
	configReloadTimer *time.Timer
	serverUpdateMu    sync.Mutex
	serverUpdateTimer *time.Timer
	serverUpdateLast  time.Time
	serverUpdatePend  bool
	stopped           atomic.Bool
	reloadCallback    func(*config.Config)
	configSource      nacos.ConfigSource
	authStore         nacos.WatchableAuthStore
	lastAuthHashes    map[string]string
	lastAuthContents  map[string]*coreauth.Auth
	fileAuthsByPath   map[string]map[string]*coreauth.Auth
	lastRemoveTimes   map[string]time.Time
	lastConfigHash    string
	authQueue         chan<- AuthUpdate
	currentAuths      map[string]*coreauth.Auth
	storeAuths        map[string]*coreauth.Auth
	runtimeAuths      map[string]*coreauth.Auth
	dispatchMu        sync.Mutex
	dispatchCond      *sync.Cond
	pendingUpdates    map[string]AuthUpdate
	pendingOrder      []string
	dispatchCancel    context.CancelFunc
	oldConfigYaml     []byte
}

// AuthUpdateAction represents the type of change detected in auth sources.
type AuthUpdateAction string

const (
	AuthUpdateActionAdd    AuthUpdateAction = "add"
	AuthUpdateActionModify AuthUpdateAction = "modify"
	AuthUpdateActionDelete AuthUpdateAction = "delete"
)

// AuthUpdate describes an incremental change to auth configuration.
type AuthUpdate struct {
	Action AuthUpdateAction
	ID     string
	Auth   *coreauth.Auth
}

const (
	// replaceCheckDelay is a short delay to allow atomic replace (rename) to settle
	// before deciding whether a Remove event indicates a real deletion.
	replaceCheckDelay        = 50 * time.Millisecond
	configReloadDebounce     = 150 * time.Millisecond
	authRemoveDebounceWindow = 1 * time.Second
	serverUpdateDebounce     = 1 * time.Second
)

// NewWatcher creates a new file watcher instance
func NewWatcher(configPath, authDir string, reloadCallback func(*config.Config), configSource nacos.ConfigSource, authStore nacos.WatchableAuthStore) (*Watcher, error) {
	w := &Watcher{
		configPath:      configPath,
		authDir:         authDir,
		reloadCallback:  reloadCallback,
		configSource:    configSource,
		authStore:       authStore,
		lastAuthHashes:  make(map[string]string),
		fileAuthsByPath: make(map[string]map[string]*coreauth.Auth),
	}
	w.dispatchCond = sync.NewCond(&w.dispatchMu)
	return w, nil
}

// Start begins watching the configuration file and authentication directory
func (w *Watcher) Start(ctx context.Context) error {
	if w.configSource != nil {
		if err := w.configSource.WatchConfig(func(cfg *config.Config) {
			if w.stopped.Load() {
				return
			}
			w.reloadConfigFromSource(cfg)
		}); err != nil {
			return err
		}
	}

	if w.authStore != nil {
		if auths, err := w.authStore.List(ctx); err != nil {
			return err
		} else {
			w.clientsMutex.Lock()
			if len(auths) == 0 {
				w.storeAuths = nil
			} else {
				w.storeAuths = make(map[string]*coreauth.Auth, len(auths))
				for _, auth := range auths {
					if auth == nil || auth.ID == "" {
						continue
					}
					w.storeAuths[auth.ID] = auth.Clone()
				}
			}
			w.clientsMutex.Unlock()
		}
		if err := w.authStore.Watch(ctx, func(auths []*coreauth.Auth) {
			if w.stopped.Load() {
				return
			}
			w.reloadAuthsFromStore(auths)
		}); err != nil {
			return err
		}
	}

	w.reloadClients(true, nil, false)
	return nil
}

// Stop stops the file watcher
func (w *Watcher) Stop() error {
	w.stopped.Store(true)
	w.stopDispatch()
	w.stopConfigReloadTimer()
	w.stopServerUpdateTimer()
	if w.configSource != nil {
		w.configSource.StopWatch()
	}
	if w.authStore != nil {
		w.authStore.StopWatch()
	}
	return nil
}

// SetConfig updates the current configuration
func (w *Watcher) SetConfig(cfg *config.Config) {
	w.clientsMutex.Lock()
	defer w.clientsMutex.Unlock()
	w.config = cfg
	w.oldConfigYaml, _ = yaml.Marshal(cfg)
}

// SetAuthUpdateQueue sets the queue used to emit auth updates.
func (w *Watcher) SetAuthUpdateQueue(queue chan<- AuthUpdate) {
	w.setAuthUpdateQueue(queue)
}

// DispatchRuntimeAuthUpdate allows external runtime providers (e.g., websocket-driven auths)
// to push auth updates through the same queue used by file/config watchers.
// Returns true if the update was enqueued; false if no queue is configured.
func (w *Watcher) DispatchRuntimeAuthUpdate(update AuthUpdate) bool {
	return w.dispatchRuntimeAuthUpdate(update)
}

// SnapshotCoreAuths converts current clients snapshot into core auth entries.
func (w *Watcher) SnapshotCoreAuths() []*coreauth.Auth {
	w.clientsMutex.RLock()
	cfg := w.config
	w.clientsMutex.RUnlock()
	return snapshotCoreAuths(cfg, w.authDir)
}
