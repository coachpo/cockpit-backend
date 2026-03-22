package cliproxy

import (
	"context"
	"os"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/watcher"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

type serviceRuntimeModeConfigSource struct {
	mode string
}

func (s serviceRuntimeModeConfigSource) LoadConfig() (*config.Config, error) {
	return &config.Config{}, nil
}
func (s serviceRuntimeModeConfigSource) SaveConfig(*config.Config) error { return nil }
func (s serviceRuntimeModeConfigSource) WatchConfig(func(*config.Config)) error {
	return nil
}
func (s serviceRuntimeModeConfigSource) StopWatch()   {}
func (s serviceRuntimeModeConfigSource) Mode() string { return s.mode }

func TestApplyRuntimeAuthDirClearsAuthDirInNacosMode(t *testing.T) {
	service := &Service{
		cfg:          &config.Config{AuthDir: "/home/qing/projects/cockpit/.sisyphus/local-start/auth"},
		configSource: serviceRuntimeModeConfigSource{mode: "nacos"},
	}

	if err := service.applyRuntimeAuthDir(); err != nil {
		t.Fatalf("applyRuntimeAuthDir(): %v", err)
	}

	if service.cfg.AuthDir != "" {
		t.Fatalf("expected nacos runtime auth dir to be empty, got %q", service.cfg.AuthDir)
	}
	if service.cfg.AuthDir == "/home/qing/projects/cockpit/.sisyphus/local-start/auth" {
		t.Fatal("expected nacos runtime auth dir override to ignore configured remote path")
	}
}

func TestEnsureAuthDirSkipsEmptyAuthDir(t *testing.T) {
	workingDir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd(): %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("os.Chdir(): %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(previousWD); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})

	service := &Service{cfg: &config.Config{AuthDir: ""}}
	if err := service.ensureAuthDir(); err != nil {
		t.Fatalf("ensureAuthDir(): %v", err)
	}

	entries, err := os.ReadDir(workingDir)
	if err != nil {
		t.Fatalf("os.ReadDir(): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty auth dir to avoid creating filesystem entries, found %d", len(entries))
	}
}

func TestWSOnConnected_SkipsAIStudioRuntimeAuth(t *testing.T) {
	t.Parallel()

	service := &Service{
		cfg:         &config.Config{},
		coreManager: coreauth.NewManager(nil, nil, nil),
	}

	service.wsOnConnected("aistudio-runtime-1")

	if auth, ok := service.coreManager.GetByID("aistudio-runtime-1"); ok && auth != nil {
		t.Fatalf("expected websocket connection to not auto-create runtime auth, got provider %q", auth.Provider)
	}
}

func TestWSOnConnected_IgnoresEmptyChannelID(t *testing.T) {
	t.Parallel()

	service := &Service{
		cfg:         &config.Config{},
		coreManager: coreauth.NewManager(nil, nil, nil),
	}

	service.wsOnConnected("")

	if auths := service.coreManager.List(); len(auths) != 0 {
		t.Fatalf("expected empty channel id to leave runtime auth state untouched, got %d auths", len(auths))
	}
}

func TestWSOnDisconnected_PreservesAuthState(t *testing.T) {
	t.Parallel()

	service := &Service{
		cfg:         &config.Config{},
		coreManager: coreauth.NewManager(nil, nil, nil),
	}

	if _, err := service.coreManager.Register(context.Background(), &coreauth.Auth{
		ID:       "aistudio-runtime-2",
		Provider: "codex",
		Status:   coreauth.StatusActive,
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	service.wsOnDisconnected("aistudio-runtime-2", nil)

	auth, ok := service.coreManager.GetByID("aistudio-runtime-2")
	if !ok || auth == nil {
		t.Fatal("expected manual auth registration to survive websocket disconnect")
	}
	if auth.Disabled {
		t.Fatal("expected websocket disconnect to preserve enabled auth state")
	}
	if auth.Status != coreauth.StatusActive {
		t.Fatalf("expected auth status to remain active, got %q", auth.Status)
	}
}

func TestHandleAuthUpdate_DeleteRemovesAuthImmediately(t *testing.T) {
	t.Parallel()

	service := &Service{
		cfg:         &config.Config{},
		coreManager: coreauth.NewManager(nil, nil, nil),
	}

	if _, err := service.coreManager.Register(context.Background(), &coreauth.Auth{
		ID:       "stale-auth",
		Provider: "codex",
		Status:   coreauth.StatusActive,
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	service.handleAuthUpdate(context.Background(), watcher.AuthUpdate{
		Action: watcher.AuthUpdateActionDelete,
		ID:     "stale-auth",
	})

	if _, ok := service.coreManager.GetByID("stale-auth"); ok {
		t.Fatal("expected deleted auth to be removed from core manager")
	}
	if auths := service.coreManager.List(); len(auths) != 0 {
		t.Fatalf("expected no auths after delete update, got %d", len(auths))
	}
}
