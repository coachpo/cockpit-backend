package cockpit

import (
	"context"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

type builderTestAuthStore struct{}

type builderTestConfigSource struct {
	mode string
}

func (s builderTestConfigSource) LoadConfig() (*config.Config, error) { return &config.Config{}, nil }
func (s builderTestConfigSource) SaveConfig(*config.Config) error     { return nil }
func (s builderTestConfigSource) WatchConfig(func(*config.Config)) error {
	return nil
}
func (s builderTestConfigSource) StopWatch()   {}
func (s builderTestConfigSource) Mode() string { return s.mode }

func (builderTestAuthStore) List(context.Context) ([]*coreauth.Auth, error) { return nil, nil }
func (builderTestAuthStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}
func (builderTestAuthStore) Delete(context.Context, string) error { return nil }
func (builderTestAuthStore) ReadByName(context.Context, string) ([]byte, error) {
	return nil, nil
}
func (builderTestAuthStore) ListMetadata(context.Context) ([]nacos.AuthFileMetadata, error) {
	return nil, nil
}
func (builderTestAuthStore) Watch(context.Context, func([]*coreauth.Auth)) error { return nil }
func (builderTestAuthStore) StopWatch()                                          {}

func TestBuildFailsWhenConfigSourceIsMissing(t *testing.T) {
	_, err := NewBuilder().
		WithConfig(&config.Config{}).
		WithAuthStore(builderTestAuthStore{}).
		Build()
	if err == nil {
		t.Fatal("expected Build to fail when config source is missing")
	}
	if !strings.Contains(err.Error(), "config source") {
		t.Fatalf("expected config source error, got %q", err)
	}
}

func TestBuildFailsWhenAuthStoreIsMissing(t *testing.T) {
	_, err := NewBuilder().
		WithConfig(&config.Config{}).
		WithConfigSource(builderTestConfigSource{mode: "nacos"}).
		Build()
	if err == nil {
		t.Fatal("expected Build to fail when auth store is missing")
	}
	if !strings.Contains(err.Error(), "auth store") {
		t.Fatalf("expected auth store error, got %q", err)
	}
}

func TestBuildAllowsNacosConfigSource(t *testing.T) {
	service, err := NewBuilder().
		WithConfig(&config.Config{}).
		WithConfigSource(builderTestConfigSource{mode: "nacos"}).
		WithAuthStore(builderTestAuthStore{}).
		Build()
	if err != nil {
		t.Fatalf("expected Build to allow Nacos config source, got %v", err)
	}
	if service == nil {
		t.Fatal("expected Build to return a service")
	}
}
