package api

import (
	"context"
	"fmt"
	"time"

	"github.com/coachpo/cockpit-backend/internal/access"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/util"
	"github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
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
func (s *Server) UpdateConfig(cfg *config.Config) {
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

	util.SetLogLevel(cfg)

	s.applyAccessConfig(oldCfg, cfg)
	s.cfg = cfg
	s.wsAuthEnabled.Store(cfg.WebsocketAuth)
	if oldCfg != nil && s.wsAuthChanged != nil && oldCfg.WebsocketAuth != cfg.WebsocketAuth {
		s.wsAuthChanged(oldCfg.WebsocketAuth, cfg.WebsocketAuth)
	}
	s.oldConfigYaml, _ = yaml.Marshal(cfg)

	s.handlers.UpdateConfig(&cfg.SDKConfig)

	if s.mgmt != nil {
		s.mgmt.SetConfig(cfg)
		s.mgmt.SetAuthManager(s.handlers.AuthManager)
	}

	authEntries := util.CountAuthFiles(context.Background(), s.authStore)
	codexAPIKeyCount := len(cfg.CodexKey)

	total := authEntries + codexAPIKeyCount
	fmt.Printf("server clients and configuration updated: %d clients (%d auth entries + %d Codex keys)\n",
		total,
		authEntries,
		codexAPIKeyCount,
	)
}

func (s *Server) SetWebsocketAuthChangeHandler(fn func(bool, bool)) {
	if s == nil {
		return
	}
	s.wsAuthChanged = fn
}
