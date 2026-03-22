// config_reload.go implements debounced configuration hot reload.
// It detects material changes and reloads clients when the config changes.
package watcher

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/util"
	"github.com/coachpo/cockpit-backend/internal/watcher/diff"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"gopkg.in/yaml.v3"

	log "github.com/sirupsen/logrus"
)

func (w *Watcher) stopConfigReloadTimer() {
	w.configReloadMu.Lock()
	if w.configReloadTimer != nil {
		w.configReloadTimer.Stop()
		w.configReloadTimer = nil
	}
	w.configReloadMu.Unlock()
}

func (w *Watcher) scheduleConfigReload() {
	w.configReloadMu.Lock()
	defer w.configReloadMu.Unlock()
	if w.configReloadTimer != nil {
		w.configReloadTimer.Stop()
	}
	w.configReloadTimer = time.AfterFunc(configReloadDebounce, func() {
		w.configReloadMu.Lock()
		w.configReloadTimer = nil
		w.configReloadMu.Unlock()
		w.reloadConfigIfChanged()
	})
}

func (w *Watcher) reloadConfigIfChanged() {
	data, err := os.ReadFile(w.configPath)
	if err != nil {
		log.Errorf("failed to read config file for hash check: %v", err)
		return
	}
	if len(data) == 0 {
		log.Debugf("ignoring empty config file write event")
		return
	}
	sum := sha256.Sum256(data)
	newHash := hex.EncodeToString(sum[:])

	w.clientsMutex.RLock()
	currentHash := w.lastConfigHash
	w.clientsMutex.RUnlock()

	if currentHash != "" && currentHash == newHash {
		log.Debugf("config file content unchanged (hash match), skipping reload")
		return
	}
	log.Infof("config file changed, reloading: %s", w.configPath)
	if w.reloadConfig() {
		finalHash := newHash
		if updatedData, errRead := os.ReadFile(w.configPath); errRead == nil && len(updatedData) > 0 {
			sumUpdated := sha256.Sum256(updatedData)
			finalHash = hex.EncodeToString(sumUpdated[:])
		} else if errRead != nil {
			log.WithError(errRead).Debug("failed to compute updated config hash after reload")
		}
		w.clientsMutex.Lock()
		w.lastConfigHash = finalHash
		w.clientsMutex.Unlock()
	}
}

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

	authDirChanged := oldConfig == nil || oldConfig.AuthDir != newConfig.AuthDir
	retryConfigChanged := oldConfig != nil && (oldConfig.RequestRetry != newConfig.RequestRetry || oldConfig.MaxRetryInterval != newConfig.MaxRetryInterval || oldConfig.MaxRetryCredentials != newConfig.MaxRetryCredentials)
	forceAuthRefresh := oldConfig != nil && retryConfigChanged

	log.Infof("config reloaded from source, triggering client reload")
	w.reloadClients(authDirChanged, forceAuthRefresh)
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
	w.reloadClients(false, true)
}

func (w *Watcher) reloadConfig() bool {
	log.Debug("=========================== CONFIG RELOAD ============================")
	log.Debugf("starting config reload from: %s", w.configPath)

	newConfig, errLoadConfig := config.LoadConfig(w.configPath)
	if errLoadConfig != nil {
		log.Errorf("failed to reload config: %v", errLoadConfig)
		return false
	}

	if resolvedAuthDir, errResolveAuthDir := util.ResolveAuthDir(newConfig.AuthDir); errResolveAuthDir != nil {
		log.Errorf("failed to resolve auth directory from config: %v", errResolveAuthDir)
	} else {
		newConfig.AuthDir = resolvedAuthDir
	}

	w.clientsMutex.Lock()
	var oldConfig *config.Config
	_ = yaml.Unmarshal(w.oldConfigYaml, &oldConfig)
	w.oldConfigYaml, _ = yaml.Marshal(newConfig)
	w.config = newConfig
	w.clientsMutex.Unlock()

	util.SetLogLevel(newConfig)

	if oldConfig != nil {
		details := diff.BuildConfigChangeDetails(oldConfig, newConfig)
		if len(details) > 0 {
			log.Debugf("config changes detected:")
			for _, d := range details {
				log.Debugf("  %s", d)
			}
		} else {
			log.Debugf("no material config field changes detected")
		}
	}

	authDirChanged := oldConfig == nil || oldConfig.AuthDir != newConfig.AuthDir
	retryConfigChanged := oldConfig != nil && (oldConfig.RequestRetry != newConfig.RequestRetry || oldConfig.MaxRetryInterval != newConfig.MaxRetryInterval || oldConfig.MaxRetryCredentials != newConfig.MaxRetryCredentials)
	forceAuthRefresh := oldConfig != nil && retryConfigChanged

	log.Infof("config successfully reloaded, triggering client reload")
	w.reloadClients(authDirChanged, forceAuthRefresh)
	return true
}
