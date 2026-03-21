package cliproxy

import (
	"context"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/watcher"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

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
