package nacos

import (
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

type stubConfigClient struct {
	config     string
	listenFunc func(namespace, group, dataID, data string)
}

func (s *stubConfigClient) GetConfig(vo.ConfigParam) (string, error) { return s.config, nil }

func (s *stubConfigClient) PublishConfig(vo.ConfigParam) (bool, error) { return true, nil }

func (s *stubConfigClient) DeleteConfig(vo.ConfigParam) (bool, error) { return true, nil }

func (s *stubConfigClient) ListenConfig(param vo.ConfigParam) error {
	s.listenFunc = param.OnChange
	return nil
}

func (s *stubConfigClient) CancelListenConfig(vo.ConfigParam) error { return nil }

func (s *stubConfigClient) SearchConfig(vo.SearchConfigParam) (*model.ConfigPage, error) {
	return nil, nil
}

func (s *stubConfigClient) CloseClient() {}

func TestNacosConfigStoreParseConfigRejectsRemovedKeys(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "removed root key",
			raw:     "port: 8080\nrequest-log: true\n",
			wantErr: "request-log",
		},
		{
			name:    "removed nested key",
			raw:     "quota-exceeded:\n  switch-project: true\n  switch-preview-model: true\n",
			wantErr: "switch-preview-model",
		},
		{
			name:    "removed remote management block",
			raw:     "port: 8080\nremote-management:\n  allow-remote: false\n",
			wantErr: "remote-management",
		},
		{
			name:    "removed codex subfield",
			raw:     "codex-api-key:\n  - api-key: test\n    base-url: https://example.invalid/v1\n    proxy-url: https://proxy.invalid\n",
			wantErr: "proxy-url",
		},
		{
			name:    "codex key without base url",
			raw:     "codex-api-key:\n  - api-key: test\n    base-url: \"   \"\n",
			wantErr: "codex-api-key[0].base-url is required",
		},
	}

	store := &NacosConfigStore{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := store.parseConfig(tt.raw)
			if err == nil {
				t.Fatal("expected parseConfig to reject removed key")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNacosConfigStoreSaveConfigRejectsCodexKeyWithoutBaseURL(t *testing.T) {
	store := &NacosConfigStore{}
	err := store.SaveConfig(&config.Config{CodexKey: []config.CodexKey{{APIKey: "test", BaseURL: "   "}}})
	if err == nil {
		t.Fatal("expected SaveConfig to reject codex key without base-url")
	}
	if !strings.Contains(err.Error(), "codex-api-key[0].base-url is required") {
		t.Fatalf("expected base-url validation error, got %v", err)
	}
}

func TestNacosConfigStoreWatchConfigIgnoresRemovedFieldOnlyUpdates(t *testing.T) {
	client := &stubConfigClient{config: "host: \"\"\nport: 8080\nauth-dir: /tmp/auth\n"}
	store := &NacosConfigStore{
		client:       &Client{group: "DEFAULT_GROUP"},
		configClient: client,
	}

	callbackCalls := 0
	if err := store.WatchConfig(func(*config.Config) {
		callbackCalls++
	}); err != nil {
		t.Fatalf("WatchConfig returned error: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("expected initial callback once, got %d", callbackCalls)
	}
	if client.listenFunc == nil {
		t.Fatal("expected ListenConfig to register an OnChange callback")
	}

	client.listenFunc("public", "DEFAULT_GROUP", nacosConfigDataID, "host: \"\"\nport: 8080\nauth-dir: /tmp/auth\nrequest-log: true\n")
	if callbackCalls != 1 {
		t.Fatalf("expected removed-field-only update to be ignored, got %d callbacks", callbackCalls)
	}
}
