package watcher

import (
	"context"
	"testing"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

func TestBuildAPIKeyClientsCounts(t *testing.T) {
	cfg := &config.Config{
		CodexKey: []config.CodexKey{
			{APIKey: "sk-1", BaseURL: "https://api.openai.com/v1"},
			{APIKey: "sk-2", BaseURL: "https://api.openai.com/v1"},
		},
	}

	if got := BuildAPIKeyClients(cfg); got != 2 {
		t.Fatalf("expected 2 API key clients, got %d", got)
	}
}

func TestNormalizeAuthStripsTemporalFields(t *testing.T) {
	now := time.Now()
	auth := &coreauth.Auth{
		ID:               "auth-1",
		Provider:         "codex",
		CreatedAt:        now,
		UpdatedAt:        now,
		LastRefreshedAt:  now,
		NextRefreshAfter: now,
		NextRetryAfter:   now,
		Runtime:          struct{}{},
		Quota: coreauth.QuotaState{
			NextRecoverAt: now,
		},
	}

	normalized := normalizeAuth(auth)
	if normalized.CreatedAt != (time.Time{}) || normalized.UpdatedAt != (time.Time{}) {
		t.Fatalf("expected created/updated timestamps to be stripped, got %+v", normalized)
	}
	if normalized.LastRefreshedAt != (time.Time{}) || normalized.NextRefreshAfter != (time.Time{}) {
		t.Fatalf("expected refresh timestamps to be stripped, got %+v", normalized)
	}
	if normalized.NextRetryAfter != now {
		t.Fatalf("expected next retry timestamp to be preserved, got %+v", normalized)
	}
	if normalized.Runtime != nil {
		t.Fatalf("expected runtime state to be stripped, got %#v", normalized.Runtime)
	}
	if normalized.Quota.NextRecoverAt != (time.Time{}) {
		t.Fatalf("expected quota next recover timestamp to be stripped, got %+v", normalized.Quota)
	}
}

func TestSnapshotCoreAuths_ConfigOnly(t *testing.T) {
	cfg := &config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:     "sk-test",
			BaseURL:    "https://api.openai.com/v1",
			Priority:   7,
			Websockets: true,
		}},
	}

	auths := snapshotCoreAuths(cfg)
	if len(auths) != 1 {
		t.Fatalf("expected 1 synthesized auth, got %d", len(auths))
	}
	auth := auths[0]
	if auth.Provider != "codex" {
		t.Fatalf("expected codex provider, got %q", auth.Provider)
	}
	if auth.Attributes["auth_kind"] != "apikey" {
		t.Fatalf("expected apikey auth_kind, got %#v", auth.Attributes)
	}
	if auth.Attributes["priority"] != "7" {
		t.Fatalf("expected priority 7, got %#v", auth.Attributes)
	}
	if auth.Attributes["websockets"] != "true" {
		t.Fatalf("expected websockets=true, got %#v", auth.Attributes)
	}
}

func TestDispatchRuntimeAuthUpdateEnqueuesAndUpdatesState(t *testing.T) {
	w, err := NewWatcher(nil, nil, nil)
	if err != nil {
		t.Fatalf("NewWatcher(): %v", err)
	}
	queue := make(chan AuthUpdate, 1)
	w.SetAuthUpdateQueue(queue)

	ok := w.DispatchRuntimeAuthUpdate(AuthUpdate{
		Action: AuthUpdateActionAdd,
		Auth: &coreauth.Auth{
			ID:       "runtime-1",
			Provider: "codex",
			Status:   coreauth.StatusActive,
		},
	})
	if !ok {
		t.Fatal("expected runtime auth update to enqueue")
	}

	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionAdd || update.Auth == nil || update.Auth.ID != "runtime-1" {
			t.Fatalf("unexpected queued update: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queued runtime auth update")
	}

	if auth := w.runtimeAuths["runtime-1"]; auth == nil {
		t.Fatal("expected runtime auth state to be updated")
	}
}

func TestReloadAuthsFromStoreDispatchesStoreAuths(t *testing.T) {
	w, err := NewWatcher(nil, nil, nil)
	if err != nil {
		t.Fatalf("NewWatcher(): %v", err)
	}
	w.SetConfig(&config.Config{})
	queue := make(chan AuthUpdate, 1)
	w.SetAuthUpdateQueue(queue)

	w.reloadAuthsFromStore([]*coreauth.Auth{{
		ID:       "store-1",
		Provider: "codex",
		Status:   coreauth.StatusActive,
	}})

	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionAdd || update.ID != "store-1" {
			t.Fatalf("unexpected queued update: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queued store auth update")
	}
}

func TestPrepareAuthUpdatesLockedForceAndDelete(t *testing.T) {
	queue := make(chan AuthUpdate, 1)
	w := &Watcher{
		authQueue: queue,
		currentAuths: map[string]*coreauth.Auth{
			"auth-1": {
				ID:       "auth-1",
				Provider: "codex",
				Status:   coreauth.StatusActive,
			},
		},
	}

	updates := w.prepareAuthUpdatesLocked([]*coreauth.Auth{{
		ID:       "auth-1",
		Provider: "codex",
		Status:   coreauth.StatusActive,
	}}, true)
	if len(updates) != 1 || updates[0].Action != AuthUpdateActionModify {
		t.Fatalf("expected forced modify update, got %+v", updates)
	}

	updates = w.prepareAuthUpdatesLocked(nil, false)
	if len(updates) != 1 || updates[0].Action != AuthUpdateActionDelete || updates[0].ID != "auth-1" {
		t.Fatalf("expected delete update, got %+v", updates)
	}
}

type startTestAuthStore struct {
	auths        []*coreauth.Auth
	watchHandler func([]*coreauth.Auth)
}

func (s *startTestAuthStore) List(context.Context) ([]*coreauth.Auth, error) {
	out := make([]*coreauth.Auth, 0, len(s.auths))
	for _, auth := range s.auths {
		out = append(out, auth.Clone())
	}
	return out, nil
}

func (s *startTestAuthStore) Save(context.Context, *coreauth.Auth) (string, error) { return "", nil }

func (s *startTestAuthStore) Delete(context.Context, string) error { return nil }

func (s *startTestAuthStore) ReadByName(context.Context, string) ([]byte, error) { return nil, nil }

func (s *startTestAuthStore) ListMetadata(context.Context) ([]nacos.AuthFileMetadata, error) {
	return nil, nil
}

func (s *startTestAuthStore) Watch(_ context.Context, onChange func([]*coreauth.Auth)) error {
	s.watchHandler = onChange
	return nil
}

func (s *startTestAuthStore) StopWatch() {}

func TestStartSeedsStoreAuthsBeforeWatchingChanges(t *testing.T) {
	store := &startTestAuthStore{auths: []*coreauth.Auth{{
		ID:       "seeded-auth",
		Provider: "codex",
		Status:   coreauth.StatusActive,
	}}}
	w, err := NewWatcher(nil, nil, store)
	if err != nil {
		t.Fatalf("NewWatcher(): %v", err)
	}
	w.SetConfig(&config.Config{})
	queue := make(chan AuthUpdate, 1)
	w.SetAuthUpdateQueue(queue)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if stopErr := w.Stop(); stopErr != nil {
			t.Fatalf("Watcher.Stop(): %v", stopErr)
		}
	}()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Watcher.Start(): %v", err)
	}

	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionAdd || update.ID != "seeded-auth" {
			t.Fatalf("unexpected queued update: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for seeded store auth update")
	}
}
