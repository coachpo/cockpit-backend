package watcher

import (
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	log "github.com/sirupsen/logrus"
)

func (w *Watcher) reloadClients(forceAuthRefresh bool) {
	log.Debugf("starting full client load process")

	w.clientsMutex.RLock()
	cfg := w.config
	storeAuthCount := len(w.storeAuths)
	runtimeAuthCount := len(w.runtimeAuths)
	w.clientsMutex.RUnlock()

	if cfg == nil {
		log.Error("config is nil, cannot reload clients")
		return
	}

	codexAPIKeyCount := BuildAPIKeyClients(cfg)
	totalAPIKeyClients := codexAPIKeyCount
	log.Debugf("loaded %d API key clients", totalAPIKeyClients)

	totalNewClients := storeAuthCount + runtimeAuthCount + codexAPIKeyCount

	if w.reloadCallback != nil {
		log.Debugf("triggering server update callback before auth refresh")
		w.reloadCallback(cfg)
	}

	w.refreshAuthState(forceAuthRefresh)

	log.Infof("full client load complete - %d auth entries (%d store auths + %d runtime auths + %d Codex keys)",
		totalNewClients,
		storeAuthCount,
		runtimeAuthCount,
		codexAPIKeyCount,
	)
}

func BuildAPIKeyClients(cfg *config.Config) int {
	codexAPIKeyCount := 0

	if len(cfg.CodexKey) > 0 {
		codexAPIKeyCount += len(cfg.CodexKey)
	}
	return codexAPIKeyCount
}

func (w *Watcher) stopServerUpdateTimer() {
	w.serverUpdateMu.Lock()
	defer w.serverUpdateMu.Unlock()
	if w.serverUpdateTimer != nil {
		w.serverUpdateTimer.Stop()
		w.serverUpdateTimer = nil
	}
	w.serverUpdatePend = false
}

func (w *Watcher) triggerServerUpdate(cfg *config.Config) {
	if w == nil || w.reloadCallback == nil || cfg == nil {
		return
	}
	if w.stopped.Load() {
		return
	}

	now := time.Now()

	w.serverUpdateMu.Lock()
	if w.serverUpdateLast.IsZero() || now.Sub(w.serverUpdateLast) >= serverUpdateDebounce {
		w.serverUpdateLast = now
		if w.serverUpdateTimer != nil {
			w.serverUpdateTimer.Stop()
			w.serverUpdateTimer = nil
		}
		w.serverUpdatePend = false
		w.serverUpdateMu.Unlock()
		w.reloadCallback(cfg)
		return
	}

	if w.serverUpdatePend {
		w.serverUpdateMu.Unlock()
		return
	}

	delay := serverUpdateDebounce - now.Sub(w.serverUpdateLast)
	if delay < 10*time.Millisecond {
		delay = 10 * time.Millisecond
	}
	w.serverUpdatePend = true
	if w.serverUpdateTimer != nil {
		w.serverUpdateTimer.Stop()
		w.serverUpdateTimer = nil
	}
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		if w.stopped.Load() {
			return
		}
		w.clientsMutex.RLock()
		latestCfg := w.config
		w.clientsMutex.RUnlock()

		w.serverUpdateMu.Lock()
		if w.serverUpdateTimer != timer || !w.serverUpdatePend {
			w.serverUpdateMu.Unlock()
			return
		}
		w.serverUpdateTimer = nil
		w.serverUpdatePend = false
		if latestCfg == nil || w.reloadCallback == nil || w.stopped.Load() {
			w.serverUpdateMu.Unlock()
			return
		}

		w.serverUpdateLast = time.Now()
		w.serverUpdateMu.Unlock()
		w.reloadCallback(latestCfg)
	})
	w.serverUpdateTimer = timer
	w.serverUpdateMu.Unlock()
}
