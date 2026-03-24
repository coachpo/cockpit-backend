package nacos

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	log "github.com/sirupsen/logrus"
)

const nacosAuthDataID = "auth-credentials"

type NacosAuthStore struct {
	client       *Client
	configClient config_client.IConfigClient

	mu sync.Mutex

	stateMu     sync.RWMutex
	lastEntries map[string]map[string]any
	lastMd5     string
}

func NewNacosAuthStore(client *Client) *NacosAuthStore {
	store := &NacosAuthStore{client: client}
	if client != nil {
		store.configClient = client.ConfigClient()
	}
	return store
}

func (s *NacosAuthStore) List(_ context.Context) ([]*coreauth.Auth, error) {
	entries, raw, err := s.loadEntries()
	if err != nil {
		return nil, err
	}

	s.stateMu.Lock()
	s.lastEntries = cloneAuthEntries(entries)
	s.lastMd5 = md5Hex(raw)
	s.stateMu.Unlock()

	return authListFromEntries(entries), nil
}

func (s *NacosAuthStore) Save(_ context.Context, auth *coreauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("nacos auth store: auth is nil")
	}

	id := strings.TrimSpace(auth.ID)
	if id == "" {
		return "", fmt.Errorf("nacos auth store: auth id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, _, err := s.loadEntries()
	if err != nil {
		return "", err
	}

	entries[id] = authToEntry(auth)
	raw, err := marshalAuthEntries(entries)
	if err != nil {
		return "", err
	}

	client, err := s.clientOrError()
	if err != nil {
		return "", err
	}

	ok, err := client.PublishConfig(vo.ConfigParam{
		DataId:  nacosAuthDataID,
		Group:   s.client.Group(),
		Type:    "json",
		Content: raw,
	})
	if err != nil {
		return "", fmt.Errorf("nacos auth store: publish auths: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("nacos auth store: publish auths returned false")
	}

	s.stateMu.Lock()
	s.lastEntries = cloneAuthEntries(entries)
	s.lastMd5 = md5Hex(raw)
	s.stateMu.Unlock()

	return id, nil
}

func (s *NacosAuthStore) Delete(_ context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("nacos auth store: auth id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, _, err := s.loadEntries()
	if err != nil {
		return err
	}
	delete(entries, id)

	raw, err := marshalAuthEntries(entries)
	if err != nil {
		return err
	}

	client, err := s.clientOrError()
	if err != nil {
		return err
	}

	ok, err := client.PublishConfig(vo.ConfigParam{
		DataId:  nacosAuthDataID,
		Group:   s.client.Group(),
		Type:    "json",
		Content: raw,
	})
	if err != nil {
		return fmt.Errorf("nacos auth store: publish auth delete: %w", err)
	}
	if !ok {
		return fmt.Errorf("nacos auth store: publish auth delete returned false")
	}

	s.stateMu.Lock()
	s.lastEntries = cloneAuthEntries(entries)
	s.lastMd5 = md5Hex(raw)
	s.stateMu.Unlock()

	return nil
}

func (s *NacosAuthStore) ReadByName(_ context.Context, name string) ([]byte, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("nacos auth store: auth name is empty")
	}
	entries, _, err := s.loadEntries()
	if err != nil {
		return nil, err
	}
	for id, entry := range entries {
		fileName := strings.TrimSpace(stringValue(entry, "file_name"))
		if fileName == "" {
			fileName = id
			if !strings.HasSuffix(strings.ToLower(fileName), ".json") {
				fileName += ".json"
			}
		}
		if fileName == name || id == name {
			return json.MarshalIndent(entry, "", "  ")
		}
	}
	return nil, fmt.Errorf("nacos auth store: auth %q not found", name)
}

func (s *NacosAuthStore) ListMetadata(_ context.Context) ([]AuthFileMetadata, error) {
	entries, _, err := s.loadEntries()
	if err != nil {
		return nil, err
	}
	items := make([]AuthFileMetadata, 0, len(entries))
	for id, entry := range entries {
		name := strings.TrimSpace(stringValue(entry, "file_name"))
		if name == "" {
			name = id
			if !strings.HasSuffix(strings.ToLower(name), ".json") {
				name += ".json"
			}
		}
		item := AuthFileMetadata{
			ID:    id,
			Name:  name,
			Type:  strings.TrimSpace(stringValue(entry, "type")),
			Email: strings.TrimSpace(stringValue(entry, "email")),
		}
		if rawNote, ok := entry["note"].(string); ok {
			item.Note = strings.TrimSpace(rawNote)
		}
		if updatedAt, ok := entry["updated_at"].(string); ok {
			if ts, errParse := time.Parse(time.RFC3339, strings.TrimSpace(updatedAt)); errParse == nil {
				item.ModTime = ts
			}
		}
		if rawPriority, ok := entry["priority"]; ok {
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
		if raw, errMarshal := json.Marshal(entry); errMarshal == nil {
			item.Size = int64(len(raw))
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}

func (s *NacosAuthStore) Watch(ctx context.Context, onChange func([]*coreauth.Auth)) error {
	if onChange == nil {
		return fmt.Errorf("nacos auth store: onChange is nil")
	}

	entries, raw, err := s.loadEntries()
	if err != nil {
		return err
	}

	s.stateMu.Lock()
	s.lastEntries = cloneAuthEntries(entries)
	s.lastMd5 = md5Hex(raw)
	s.stateMu.Unlock()

	if ctx != nil {
		go func() {
			<-ctx.Done()
			s.StopWatch()
		}()
	}

	client, err := s.clientOrError()
	if err != nil {
		return err
	}

	err = client.ListenConfig(vo.ConfigParam{
		DataId: nacosAuthDataID,
		Group:  s.client.Group(),
		OnChange: func(namespace, group, dataID, data string) {
			entries, errParse := parseAuthEntries(data)
			if errParse != nil {
				log.WithError(errParse).Warn("nacos auth store: ignore invalid updated auths")
				return
			}

			checksum := md5Hex(data)

			s.stateMu.Lock()
			if s.lastMd5 == checksum {
				s.stateMu.Unlock()
				return
			}

			oldEntries := s.lastEntries
			if authEntriesEqual(oldEntries, entries) {
				s.lastEntries = cloneAuthEntries(entries)
				s.lastMd5 = checksum
				s.stateMu.Unlock()
				return
			}

			s.lastEntries = cloneAuthEntries(entries)
			s.lastMd5 = checksum
			s.stateMu.Unlock()

			onChange(authListFromEntries(entries))
		},
	})
	if err != nil {
		return fmt.Errorf("nacos auth store: listen auths: %w", err)
	}

	return nil
}

func (s *NacosAuthStore) StopWatch() {
	if s == nil || s.client == nil || s.configClient == nil {
		return
	}
	if err := s.configClient.CancelListenConfig(vo.ConfigParam{DataId: nacosAuthDataID, Group: s.client.Group()}); err != nil {
		log.WithError(err).Warn("nacos auth store: cancel listen failed")
	}
}
