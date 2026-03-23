package cliproxy

import (
	"context"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

type builderTestAuthStore struct{}

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
		WithConfigPath("/tmp/config.yaml").
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
		WithConfigPath("/tmp/config.yaml").
		WithConfigSource(serviceRuntimeModeConfigSource{mode: "nacos"}).
		Build()
	if err == nil {
		t.Fatal("expected Build to fail when auth store is missing")
	}
	if !strings.Contains(err.Error(), "auth store") {
		t.Fatalf("expected auth store error, got %q", err)
	}
}
