// Package watcher watches config/auth changes and triggers hot reloads.
package watcher

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"gopkg.in/yaml.v3"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

type Watcher struct {
	config            *config.Config
	clientsMutex      sync.RWMutex
	serverUpdateMu    sync.Mutex
	serverUpdateTimer *time.Timer
	serverUpdateLast  time.Time
	serverUpdatePend  bool
	stopped           atomic.Bool
	reloadCallback    func(*config.Config)
	configSource      nacos.ConfigSource
	authStore         nacos.WatchableAuthStore
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
	serverUpdateDebounce = 1 * time.Second
)

func NewWatcher(reloadCallback func(*config.Config), configSource nacos.ConfigSource, authStore nacos.WatchableAuthStore) (*Watcher, error) {
	w := &Watcher{
		reloadCallback: reloadCallback,
		configSource:   configSource,
		authStore:      authStore,
	}
	w.dispatchCond = sync.NewCond(&w.dispatchMu)
	return w, nil
}

func (w *Watcher) sourceMode() string {
	if w == nil || w.configSource == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(w.configSource.Mode()))
}

func (w *Watcher) usesNacosSource() bool {
	return w.sourceMode() == "nacos"
}

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

	w.reloadClients(false)
	return nil
}

func (w *Watcher) Stop() error {
	w.stopped.Store(true)
	w.stopDispatch()
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

func (w *Watcher) DispatchRuntimeAuthUpdate(update AuthUpdate) bool {
	return w.dispatchRuntimeAuthUpdate(update)
}

// SnapshotCoreAuths converts current clients snapshot into core auth entries.
func (w *Watcher) SnapshotCoreAuths() []*coreauth.Auth {
	w.clientsMutex.RLock()
	cfg := w.config
	w.clientsMutex.RUnlock()
	return snapshotCoreAuths(cfg)
}
