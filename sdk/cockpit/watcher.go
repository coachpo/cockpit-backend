package cockpit

import (
	"context"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"github.com/coachpo/cockpit-backend/internal/watcher"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

func defaultWatcherFactory(reload func(*config.Config), configSource nacos.ConfigSource, authStore nacos.WatchableAuthStore) (*WatcherWrapper, error) {
	w, err := watcher.NewWatcher(reload, configSource, authStore)
	if err != nil {
		return nil, err
	}

	return &WatcherWrapper{
		start: func(ctx context.Context) error {
			return w.Start(ctx)
		},
		stop: func() error {
			return w.Stop()
		},
		setConfig: func(cfg *config.Config) {
			w.SetConfig(cfg)
		},
		snapshotAuths: func() []*coreauth.Auth { return w.SnapshotCoreAuths() },
		setUpdateQueue: func(queue chan<- watcher.AuthUpdate) {
			w.SetAuthUpdateQueue(queue)
		},
		dispatchRuntimeUpdate: func(update watcher.AuthUpdate) bool {
			return w.DispatchRuntimeAuthUpdate(update)
		},
	}, nil
}
