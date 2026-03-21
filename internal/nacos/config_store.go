package nacos

import (
	"crypto/md5"
	"fmt"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/coachpo/cockpit-backend/internal/config"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const nacosConfigDataID = "proxy-config"

type NacosConfigStore struct {
	client       *Client
	configClient config_client.IConfigClient

	mu         sync.RWMutex
	lastConfig *config.Config
	lastMd5    string
}

func NewNacosConfigStore(client *Client) *NacosConfigStore {
	store := &NacosConfigStore{client: client}
	if client != nil {
		store.configClient = client.ConfigClient()
	}
	return store
}

func (s *NacosConfigStore) LoadConfig() (*config.Config, error) {
	raw, err := s.getConfig()
	if err != nil {
		return nil, err
	}

	cfg, checksum, err := s.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.lastConfig = cfg
	s.lastMd5 = checksum
	s.mu.Unlock()

	return cfg, nil
}

func (s *NacosConfigStore) SaveConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("nacos config store: config is nil")
	}

	persistCfg, err := cloneConfig(cfg)
	if err != nil {
		return err
	}
	if err = sanitizeConfig(persistCfg, true); err != nil {
		return err
	}

	raw, err := yaml.Marshal(persistCfg)
	if err != nil {
		return fmt.Errorf("nacos config store: marshal config: %w", err)
	}

	client, err := s.clientOrError()
	if err != nil {
		return err
	}

	ok, err := client.PublishConfig(vo.ConfigParam{
		DataId:  nacosConfigDataID,
		Group:   s.client.Group(),
		Type:    "yaml",
		Content: string(raw),
	})
	if err != nil {
		return fmt.Errorf("nacos config store: publish config: %w", err)
	}
	if !ok {
		return fmt.Errorf("nacos config store: publish config returned false")
	}

	s.mu.Lock()
	s.lastConfig = persistCfg
	s.lastMd5 = md5Hex(string(raw))
	s.mu.Unlock()

	return nil
}

func (s *NacosConfigStore) WatchConfig(onChange func(*config.Config)) error {
	if onChange == nil {
		return fmt.Errorf("nacos config store: onChange is nil")
	}

	raw, err := s.getConfig()
	if err != nil {
		return err
	}

	cfg, checksum, err := s.parseConfig(raw)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.lastConfig = cfg
	s.lastMd5 = checksum
	s.mu.Unlock()

	onChange(cfg)

	client, err := s.clientOrError()
	if err != nil {
		return err
	}

	err = client.ListenConfig(vo.ConfigParam{
		DataId: nacosConfigDataID,
		Group:  s.client.Group(),
		OnChange: func(namespace, group, dataID, data string) {
			updatedCfg, updatedMd5, errParse := s.parseConfig(data)
			if errParse != nil {
				log.WithError(errParse).Warn("nacos config store: ignore invalid updated config")
				return
			}

			s.mu.Lock()
			if s.lastMd5 == updatedMd5 {
				s.mu.Unlock()
				return
			}
			s.lastConfig = updatedCfg
			s.lastMd5 = updatedMd5
			s.mu.Unlock()

			onChange(updatedCfg)
		},
	})
	if err != nil {
		return fmt.Errorf("nacos config store: listen config: %w", err)
	}

	return nil
}

func (s *NacosConfigStore) StopWatch() {
	if s == nil || s.client == nil || s.configClient == nil {
		return
	}
	if err := s.configClient.CancelListenConfig(vo.ConfigParam{DataId: nacosConfigDataID, Group: s.client.Group()}); err != nil {
		log.WithError(err).Warn("nacos config store: cancel listen failed")
	}
}

func (s *NacosConfigStore) Mode() string { return "nacos" }

func (s *NacosConfigStore) getConfig() (string, error) {
	client, err := s.clientOrError()
	if err != nil {
		return "", err
	}

	return client.GetConfig(vo.ConfigParam{
		DataId: nacosConfigDataID,
		Group:  s.client.Group(),
	})
}

func (s *NacosConfigStore) parseConfig(raw string) (*config.Config, string, error) {
	cfg := &config.Config{Host: "", DisableCooling: false}
	if err := yaml.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, "", fmt.Errorf("nacos config store: unmarshal config: %w", err)
	}
	if err := sanitizeConfig(cfg, true); err != nil {
		return nil, "", err
	}
	return cfg, md5Hex(raw), nil
}

func (s *NacosConfigStore) clientOrError() (config_client.IConfigClient, error) {
	if s == nil || s.client == nil || s.configClient == nil {
		return nil, fmt.Errorf("nacos config store: client is nil")
	}
	return s.configClient, nil
}

func cloneConfig(cfg *config.Config) (*config.Config, error) {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("nacos config store: clone config marshal: %w", err)
	}

	clone := &config.Config{}
	if err = yaml.Unmarshal(raw, clone); err != nil {
		return nil, fmt.Errorf("nacos config store: clone config unmarshal: %w", err)
	}
	return clone, nil
}

func sanitizeConfig(cfg *config.Config, hashRemoteSecret bool) error {
	if cfg == nil {
		return fmt.Errorf("nacos config store: config is nil")
	}

	if hashRemoteSecret && cfg.RemoteManagement.SecretKey != "" && !looksLikeBcryptHash(cfg.RemoteManagement.SecretKey) {
		hashed, err := hashSecretValue(cfg.RemoteManagement.SecretKey)
		if err != nil {
			return fmt.Errorf("nacos config store: hash remote management key: %w", err)
		}
		cfg.RemoteManagement.SecretKey = hashed
	}

	if cfg.MaxRetryCredentials < 0 {
		cfg.MaxRetryCredentials = 0
	}

	cfg.SanitizeCodexKeys()
	cfg.SanitizeCodexHeaderDefaults()
	cfg.SanitizeOpenAICompatibility()
	cfg.OAuthExcludedModels = config.NormalizeOAuthExcludedModels(cfg.OAuthExcludedModels)
	cfg.SanitizeOAuthModelAlias()
	cfg.SanitizePayloadRules()

	return nil
}

func md5Hex(raw string) string {
	sum := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

func looksLikeBcryptHash(value string) bool {
	return len(value) > 4 && (value[:4] == "$2a$" || value[:4] == "$2b$" || value[:4] == "$2y$")
}

func hashSecretValue(secret string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}
