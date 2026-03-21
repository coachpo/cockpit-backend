package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

type modeTestConfigSource struct {
	mode string
}

func (s modeTestConfigSource) LoadConfig() (*config.Config, error) { return &config.Config{}, nil }
func (s modeTestConfigSource) SaveConfig(*config.Config) error     { return nil }
func (s modeTestConfigSource) WatchConfig(func(*config.Config)) error {
	return nil
}
func (s modeTestConfigSource) StopWatch()   {}
func (s modeTestConfigSource) Mode() string { return s.mode }

type modeTestAuthStore struct{}

func (modeTestAuthStore) List(context.Context) ([]*coreauth.Auth, error) { return nil, nil }
func (modeTestAuthStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}
func (modeTestAuthStore) Delete(context.Context, string) error { return nil }
func (modeTestAuthStore) ReadByName(context.Context, string) ([]byte, error) {
	return nil, nil
}
func (modeTestAuthStore) ListMetadata(context.Context) ([]nacos.AuthFileMetadata, error) {
	return nil, nil
}
func (modeTestAuthStore) Watch(context.Context, func([]*coreauth.Auth)) error { return nil }
func (modeTestAuthStore) StopWatch()                                          {}

func TestWarnIfUsingLocalStaticMode_LogsWarning(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "local static mode"},
		{name: "cloud deploy static mode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			previousOutput := log.StandardLogger().Out
			previousFormatter := log.StandardLogger().Formatter
			previousLevel := log.GetLevel()
			log.SetOutput(&buffer)
			log.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
			log.SetLevel(log.WarnLevel)
			t.Cleanup(func() {
				log.SetOutput(previousOutput)
				log.SetFormatter(previousFormatter)
				log.SetLevel(previousLevel)
			})

			warnIfUsingLocalStaticMode(nacos.NewStaticConfigSource("/tmp/config.yaml"), "/tmp/config.yaml")

			output := buffer.String()
			if !strings.Contains(output, "local static file mode") {
				t.Fatalf("expected static-mode warning log, got %q", output)
			}
			if !strings.Contains(output, "/tmp/config.yaml") {
				t.Fatalf("expected warning to mention config path, got %q", output)
			}
			if !strings.Contains(output, "NACOS_ADDR") {
				t.Fatalf("expected warning to point users to NACOS_ADDR, got %q", output)
			}
		})
	}
}

func TestWarnIfUsingLocalStaticMode_SkipsNonLocalStaticModes(t *testing.T) {
	tests := []struct {
		name         string
		configSource nacos.ConfigSource
	}{
		{
			name:         "nacos mode",
			configSource: modeTestConfigSource{mode: "nacos"},
		},
		{
			name:         "nil config source",
			configSource: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			previousOutput := log.StandardLogger().Out
			previousFormatter := log.StandardLogger().Formatter
			previousLevel := log.GetLevel()
			log.SetOutput(&buffer)
			log.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
			log.SetLevel(log.WarnLevel)
			t.Cleanup(func() {
				log.SetOutput(previousOutput)
				log.SetFormatter(previousFormatter)
				log.SetLevel(previousLevel)
			})

			warnIfUsingLocalStaticMode(tc.configSource, "/tmp/config.yaml")

			if got := buffer.String(); got != "" {
				t.Fatalf("expected no warning log, got %q", got)
			}
		})
	}
}
