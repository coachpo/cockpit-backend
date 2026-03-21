package api

import (
	"context"
	"fmt"
	"time"

	"github.com/coachpo/cockpit-backend/internal/access"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/util"
	"github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func (s *Server) applyAccessConfig(oldCfg, newCfg *config.Config) {
	if s == nil || s.accessManager == nil || newCfg == nil {
		return
	}
	if _, err := access.ApplyAccessProviders(s.accessManager, oldCfg, newCfg); err != nil {
		return
	}
}

// UpdateClients applies refreshed config and client state to the live server.
func (s *Server) UpdateClients(cfg *config.Config) {
	var oldCfg *config.Config
	if len(s.oldConfigYaml) > 0 {
		_ = yaml.Unmarshal(s.oldConfigYaml, &oldCfg)
	}

	if oldCfg == nil || oldCfg.DisableCooling != cfg.DisableCooling {
		auth.SetQuotaCooldownDisabled(cfg.DisableCooling)
	}

	if s.handlers != nil && s.handlers.AuthManager != nil {
		s.handlers.AuthManager.SetRetryConfig(cfg.RequestRetry, time.Duration(cfg.MaxRetryInterval)*time.Second, cfg.MaxRetryCredentials)
	}

	if oldCfg == nil || oldCfg.Debug != cfg.Debug {
		util.SetLogLevel(cfg)
	}

	prevSecretEmpty := true
	if oldCfg != nil {
		prevSecretEmpty = oldCfg.RemoteManagement.SecretKey == ""
	}
	newSecretEmpty := cfg.RemoteManagement.SecretKey == ""
	if s.envManagementSecret {
		s.registerManagementRoutes()
		if s.managementRoutesEnabled.CompareAndSwap(false, true) {
			log.Info("management routes enabled via MANAGEMENT_PASSWORD")
		} else {
			s.managementRoutesEnabled.Store(true)
		}
	} else {
		switch {
		case prevSecretEmpty && !newSecretEmpty:
			s.registerManagementRoutes()
			if s.managementRoutesEnabled.CompareAndSwap(false, true) {
				log.Info("management routes enabled after secret key update")
			} else {
				s.managementRoutesEnabled.Store(true)
			}
		case !prevSecretEmpty && newSecretEmpty:
			if s.managementRoutesEnabled.CompareAndSwap(true, false) {
				log.Info("management routes disabled after secret key removal")
			} else {
				s.managementRoutesEnabled.Store(false)
			}
		default:
			s.managementRoutesEnabled.Store(!newSecretEmpty)
		}
	}

	s.applyAccessConfig(oldCfg, cfg)
	s.cfg = cfg
	s.wsAuthEnabled.Store(cfg.WebsocketAuth)
	if oldCfg != nil && s.wsAuthChanged != nil && oldCfg.WebsocketAuth != cfg.WebsocketAuth {
		s.wsAuthChanged(oldCfg.WebsocketAuth, cfg.WebsocketAuth)
	}
	s.oldConfigYaml, _ = yaml.Marshal(cfg)

	s.handlers.UpdateClients(&cfg.SDKConfig)

	if s.mgmt != nil {
		s.mgmt.SetConfig(cfg)
		s.mgmt.SetAuthManager(s.handlers.AuthManager)
	}

	authEntries := util.CountAuthFiles(context.Background(), s.authStore)
	codexAPIKeyCount := len(cfg.CodexKey)
	openAICompatCount := 0
	for i := range cfg.OpenAICompatibility {
		entry := cfg.OpenAICompatibility[i]
		openAICompatCount += len(entry.APIKeyEntries)
	}

	total := authEntries + codexAPIKeyCount + openAICompatCount
	fmt.Printf("server clients and configuration updated: %d clients (%d auth entries + %d Codex keys + %d OpenAI-compat)\n",
		total,
		authEntries,
		codexAPIKeyCount,
		openAICompatCount,
	)
}

func (s *Server) SetWebsocketAuthChangeHandler(fn func(bool, bool)) {
	if s == nil {
		return
	}
	s.wsAuthChanged = fn
}
