package nacos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"gopkg.in/yaml.v3"
)

type StaticConfigSource struct {
	path     string
	config   *config.Config
	mu       sync.RWMutex
	onChange func(*config.Config)
}

func NewStaticConfigSource(path string) *StaticConfigSource {
	return &StaticConfigSource{path: path}
}

func (s *StaticConfigSource) LoadConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig(s.path)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	return cfg, nil
}

func (s *StaticConfigSource) SaveConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("static config store: config is nil")
	}
	if strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("static config store: path is empty")
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
		return fmt.Errorf("static config store: marshal config: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("static config store: ensure config directory: %w", err)
	}
	if err = os.WriteFile(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("static config store: write config: %w", err)
	}

	s.mu.Lock()
	s.config = persistCfg
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange(persistCfg)
	}
	return nil
}

func (s *StaticConfigSource) WatchConfig(onChange func(*config.Config)) error {
	s.mu.Lock()
	s.onChange = onChange
	current := s.config
	s.mu.Unlock()
	if onChange != nil && current != nil {
		onChange(current)
	}
	return nil
}

func (s *StaticConfigSource) StopWatch() {
	s.mu.Lock()
	s.onChange = nil
	s.mu.Unlock()
}

func (s *StaticConfigSource) Mode() string { return "static" }

type StaticAuthStore struct {
	authDir string
}

func NewStaticAuthStore(authDir string) *StaticAuthStore {
	return &StaticAuthStore{authDir: authDir}
}

func stringValue(metadata map[string]any, key string) string {
	if len(metadata) == 0 || key == "" {
		return ""
	}
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

func (s *StaticAuthStore) List(_ context.Context) ([]*coreauth.Auth, error) {
	if s.authDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(s.authDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var auths []*coreauth.Auth
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(s.authDir, entry.Name())
		data, errRead := os.ReadFile(path)
		if errRead != nil {
			continue
		}

		var metadata map[string]any
		if errJSON := json.Unmarshal(data, &metadata); errJSON != nil {
			continue
		}

		provider, _ := metadata["type"].(string)
		email, _ := metadata["email"].(string)
		disabled, _ := metadata["disabled"].(bool)
		attributes := map[string]string{
			"path":          path,
			"store_managed": "true",
		}
		if prefix := strings.TrimSpace(stringValue(metadata, "prefix")); prefix != "" {
			attributes["prefix"] = prefix
		}
		if proxyURL := strings.TrimSpace(stringValue(metadata, "proxy_url")); proxyURL != "" {
			attributes["proxy_url"] = proxyURL
		}
		if note := strings.TrimSpace(stringValue(metadata, "note")); note != "" {
			attributes["note"] = note
		}
		if rawPriority, ok := metadata["priority"]; ok {
			switch value := rawPriority.(type) {
			case float64:
				attributes["priority"] = strconv.Itoa(int(value))
			case int:
				attributes["priority"] = strconv.Itoa(value)
			case string:
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					attributes["priority"] = trimmed
				}
			}
		}
		auth := &coreauth.Auth{
			ID:         strings.TrimSuffix(entry.Name(), ".json"),
			FileName:   entry.Name(),
			Provider:   provider,
			Label:      email,
			Prefix:     strings.TrimSpace(stringValue(metadata, "prefix")),
			ProxyURL:   strings.TrimSpace(stringValue(metadata, "proxy_url")),
			Metadata:   metadata,
			Attributes: attributes,
		}
		if disabled {
			auth.Status = coreauth.StatusDisabled
			auth.Disabled = true
		} else {
			auth.Status = coreauth.StatusActive
		}
		auths = append(auths, auth)
	}

	return auths, nil
}

func (s *StaticAuthStore) ReadByName(_ context.Context, name string) ([]byte, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(s.authDir, name))
}

func (s *StaticAuthStore) ListMetadata(_ context.Context) ([]AuthFileMetadata, error) {
	if s.authDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(s.authDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]AuthFileMetadata, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		full := filepath.Join(s.authDir, entry.Name())
		data, errRead := os.ReadFile(full)
		if errRead != nil {
			continue
		}
		var metadata map[string]any
		if errJSON := json.Unmarshal(data, &metadata); errJSON != nil {
			continue
		}
		item := AuthFileMetadata{
			ID:      strings.TrimSuffix(entry.Name(), ".json"),
			Name:    entry.Name(),
			Type:    strings.TrimSpace(stringValue(metadata, "type")),
			Email:   strings.TrimSpace(stringValue(metadata, "email")),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Source:  "file",
		}
		if rawPriority, ok := metadata["priority"]; ok {
			switch v := rawPriority.(type) {
			case float64:
				pv := int(v)
				item.Priority = &pv
			case int:
				pv := v
				item.Priority = &pv
			case string:
				if parsed, errAtoi := strconv.Atoi(strings.TrimSpace(v)); errAtoi == nil {
					pv := parsed
					item.Priority = &pv
				}
			}
		}
		if rawNote, ok := metadata["note"].(string); ok {
			item.Note = strings.TrimSpace(rawNote)
		}
		files = append(files, item)
	}
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	return files, nil
}

func (s *StaticAuthStore) Save(_ context.Context, auth *coreauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("static auth store: auth is nil")
	}
	if strings.TrimSpace(s.authDir) == "" {
		return "", fmt.Errorf("static auth store: auth dir is empty")
	}

	name := strings.TrimSpace(filepath.Base(auth.FileName))
	if name == "" {
		name = strings.TrimSpace(filepath.Base(auth.ID))
	}
	if name == "" {
		return "", fmt.Errorf("static auth store: auth file name is empty")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}

	payload := make(map[string]any)
	for key, value := range auth.Metadata {
		payload[key] = value
	}
	if provider := strings.TrimSpace(auth.Provider); provider != "" {
		payload["type"] = provider
	}
	if email := strings.TrimSpace(stringValue(payload, "email")); email == "" && strings.TrimSpace(auth.Label) != "" {
		payload["email"] = strings.TrimSpace(auth.Label)
	}
	if prefix := strings.TrimSpace(auth.Prefix); prefix != "" {
		payload["prefix"] = prefix
	} else {
		delete(payload, "prefix")
	}
	if proxyURL := strings.TrimSpace(auth.ProxyURL); proxyURL != "" {
		payload["proxy_url"] = proxyURL
	} else {
		delete(payload, "proxy_url")
	}
	if auth.Disabled {
		payload["disabled"] = true
	} else {
		delete(payload, "disabled")
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("static auth store: marshal auth payload: %w", err)
	}

	if err = os.MkdirAll(s.authDir, 0o700); err != nil {
		return "", fmt.Errorf("static auth store: ensure auth directory: %w", err)
	}
	path := filepath.Join(s.authDir, name)
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		return "", fmt.Errorf("static auth store: write auth file: %w", err)
	}
	return path, nil
}

func (s *StaticAuthStore) Delete(_ context.Context, id string) error {
	if strings.TrimSpace(s.authDir) == "" {
		return fmt.Errorf("static auth store: auth dir is empty")
	}
	name := strings.TrimSpace(filepath.Base(id))
	if name == "" {
		return fmt.Errorf("static auth store: auth id is empty")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	path := filepath.Join(s.authDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("static auth store: delete auth file: %w", err)
	}
	return nil
}

func (s *StaticAuthStore) Watch(_ context.Context, _ func([]*coreauth.Auth)) error { return nil }

func (s *StaticAuthStore) StopWatch() {}
