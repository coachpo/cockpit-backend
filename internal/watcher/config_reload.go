package watcher

import (
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/util"
	"github.com/coachpo/cockpit-backend/internal/watcher/diff"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
	"gopkg.in/yaml.v3"

	log "github.com/sirupsen/logrus"
)

func (w *Watcher) reloadConfigFromSource(newConfig *config.Config) {
	if newConfig == nil {
		return
	}
	log.Debug("=========================== CONFIG RELOAD (from source) ============================")

	var oldConfig *config.Config
	w.clientsMutex.RLock()
	_ = yaml.Unmarshal(w.oldConfigYaml, &oldConfig)
	w.clientsMutex.RUnlock()

	var details []string
	if oldConfig != nil {
		details = diff.BuildConfigChangeDetails(oldConfig, newConfig)
		if len(details) == 0 {
			log.Debug("no material config field changes detected from source")
			return
		}
	}

	w.clientsMutex.Lock()
	w.oldConfigYaml, _ = yaml.Marshal(newConfig)
	w.config = newConfig
	w.clientsMutex.Unlock()

	util.SetLogLevel(newConfig)

	if len(details) > 0 {
		log.Debugf("config changes detected:")
		for _, d := range details {
			log.Debugf("  %s", d)
		}
	}

	retryConfigChanged := oldConfig != nil && (oldConfig.RequestRetry != newConfig.RequestRetry || oldConfig.MaxRetryInterval != newConfig.MaxRetryInterval || oldConfig.MaxRetryCredentials != newConfig.MaxRetryCredentials)
	forceAuthRefresh := oldConfig != nil && retryConfigChanged

	log.Infof("config reloaded from source, triggering client reload")
	w.reloadClients(forceAuthRefresh)
}

func (w *Watcher) reloadAuthsFromStore(auths []*coreauth.Auth) {
	log.Infof("auth store changed, %d entries received", len(auths))
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
	w.reloadClients(true)
}
