package nacos

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

func stringValue(metadata map[string]any, key string) string {
	if len(metadata) == 0 || key == "" {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func authFromEntry(id string, entry map[string]any) *coreauth.Auth {
	provider, _ := entry["type"].(string)
	email, _ := entry["email"].(string)
	disabled, _ := entry["disabled"].(bool)
	auth := &coreauth.Auth{
		ID:         id,
		FileName:   strings.TrimSpace(stringValue(entry, "file_name")),
		Provider:   provider,
		Label:      email,
		Metadata:   entry,
		Attributes: map[string]string{},
	}
	if disabled {
		auth.Status = coreauth.StatusDisabled
		auth.Disabled = true
	} else {
		auth.Status = coreauth.StatusActive
	}
	for _, key := range []string{"prefix", "proxy_url", "note", "priority", "plan_type", "account_id"} {
		if v, ok := entry[key].(string); ok && v != "" {
			auth.Attributes[key] = v
		}
	}
	auth.Attributes["store_managed"] = "true"
	if prefix, ok := entry["prefix"].(string); ok {
		auth.Prefix = prefix
	}
	if proxyURL, ok := entry["proxy_url"].(string); ok {
		auth.ProxyURL = proxyURL
	}
	return auth
}

func authToEntry(auth *coreauth.Auth) map[string]any {
	if auth.Metadata != nil {
		entry := make(map[string]any, len(auth.Metadata)+4)
		for k, v := range auth.Metadata {
			entry[k] = v
		}
		if auth.Provider != "" {
			entry["type"] = auth.Provider
		}
		if auth.FileName != "" {
			entry["file_name"] = auth.FileName
		}
		if auth.Label != "" {
			entry["email"] = auth.Label
		}
		if auth.Prefix != "" {
			entry["prefix"] = auth.Prefix
		}
		if auth.ProxyURL != "" {
			entry["proxy_url"] = auth.ProxyURL
		}
		entry["disabled"] = auth.Disabled
		return entry
	}
	entry := map[string]any{
		"type":     auth.Provider,
		"email":    auth.Label,
		"disabled": auth.Disabled,
	}
	if auth.FileName != "" {
		entry["file_name"] = auth.FileName
	}
	if auth.Prefix != "" {
		entry["prefix"] = auth.Prefix
	}
	if auth.ProxyURL != "" {
		entry["proxy_url"] = auth.ProxyURL
	}
	return entry
}

func (s *NacosAuthStore) loadEntries() (map[string]map[string]any, string, error) {
	client, err := s.clientOrError()
	if err != nil {
		return nil, "", err
	}

	raw, err := client.GetConfig(vo.ConfigParam{DataId: nacosAuthDataID, Group: s.client.Group()})
	if err != nil {
		return nil, "", fmt.Errorf("nacos auth store: get auths: %w", err)
	}

	entries, err := parseAuthEntries(raw)
	if err != nil {
		return nil, "", fmt.Errorf("nacos auth store: parse auths: %w", err)
	}
	return entries, raw, nil
}

func parseAuthEntries(raw string) (map[string]map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]map[string]any{}, nil
	}

	entries := make(map[string]map[string]any)
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		return map[string]map[string]any{}, nil
	}
	normalized := make(map[string]map[string]any, len(entries))
	for id, entry := range entries {
		normalizedID, err := normalizeExplicitAuthEntryID(id)
		if err != nil {
			return nil, fmt.Errorf("nacos auth store: invalid auth id %q: %w", id, err)
		}
		if _, err := requiredAuthFileName(entry, normalizedID); err != nil {
			return nil, err
		}
		if _, exists := normalized[normalizedID]; exists {
			return nil, fmt.Errorf("nacos auth store: duplicate auth id %q", normalizedID)
		}
		normalized[normalizedID] = cloneAuthEntry(entry)
	}
	return normalized, nil
}

func requiredAuthFileName(entry map[string]any, id string) (string, error) {
	fileName := strings.TrimSpace(stringValue(entry, "file_name"))
	if fileName == "" {
		return "", fmt.Errorf("nacos auth store: auth %q is missing file_name", id)
	}
	validated, err := normalizeExplicitAuthEntryID(fileName)
	if err != nil {
		return "", fmt.Errorf("nacos auth store: auth %q has invalid file_name: %w", id, err)
	}
	return validated, nil
}

func normalizeExplicitAuthEntryID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return "", fmt.Errorf("path separators are not allowed")
	}
	return value, nil
}

func marshalAuthEntries(entries map[string]map[string]any) (string, error) {
	if entries == nil {
		entries = map[string]map[string]any{}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("nacos auth store: marshal auths: %w", err)
	}
	return string(raw), nil
}

func authListFromEntries(entries map[string]map[string]any) []*coreauth.Auth {
	if len(entries) == 0 {
		return nil
	}

	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	auths := make([]*coreauth.Auth, 0, len(ids))
	for _, id := range ids {
		auths = append(auths, authFromEntry(id, cloneAuthEntry(entries[id])))
	}
	return auths
}

func cloneAuthEntries(entries map[string]map[string]any) map[string]map[string]any {
	if len(entries) == 0 {
		return map[string]map[string]any{}
	}
	cloned := make(map[string]map[string]any, len(entries))
	for id, entry := range entries {
		cloned[id] = cloneAuthEntry(entry)
	}
	return cloned
}

func cloneAuthEntry(entry map[string]any) map[string]any {
	if len(entry) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(entry))
	for key, value := range entry {
		cloned[key] = value
	}
	return cloned
}

func authEntriesEqual(left, right map[string]map[string]any) bool {
	leftRaw, errLeft := marshalAuthEntries(left)
	rightRaw, errRight := marshalAuthEntries(right)
	if errLeft != nil || errRight != nil {
		return false
	}
	return leftRaw == rightRaw
}

func (s *NacosAuthStore) clientOrError() (config_client.IConfigClient, error) {
	if s == nil || s.client == nil || s.configClient == nil {
		return nil, fmt.Errorf("nacos auth store: client is nil")
	}
	return s.configClient, nil
}
