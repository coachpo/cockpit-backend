package watcher

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"gopkg.in/yaml.v3"
)

func TestBuildAPIKeyClientsCounts(t *testing.T) {
	cfg := &config.Config{
		CodexKey: []config.CodexKey{
			{APIKey: "x1", BaseURL: "https://api.example.invalid/v1"},
			{APIKey: "x2", BaseURL: "https://api-2.example.invalid/v1"},
		},
	}

	codex := BuildAPIKeyClients(cfg)
	if codex != 2 {
		t.Fatalf("unexpected count: %d", codex)
	}
}

func TestNormalizeAuthStripsTemporalFields(t *testing.T) {
	now := time.Now()
	auth := &coreauth.Auth{
		CreatedAt:        now,
		UpdatedAt:        now,
		LastRefreshedAt:  now,
		NextRefreshAfter: now,
		Quota: coreauth.QuotaState{
			NextRecoverAt: now,
		},
		Runtime: map[string]any{"k": "v"},
	}

	normalized := normalizeAuth(auth)
	if !normalized.CreatedAt.IsZero() || !normalized.UpdatedAt.IsZero() || !normalized.LastRefreshedAt.IsZero() || !normalized.NextRefreshAfter.IsZero() {
		t.Fatal("expected time fields to be zeroed")
	}
	if normalized.Runtime != nil {
		t.Fatal("expected runtime to be nil")
	}
	if !normalized.Quota.NextRecoverAt.IsZero() {
		t.Fatal("expected quota.NextRecoverAt to be zeroed")
	}
}

func TestSnapshotCoreAuths_ConfigAndAuthFiles(t *testing.T) {
	authDir := t.TempDir()
	metadata := map[string]any{
		"type":  "codex",
		"email": "user@example.com",
	}
	authFile := filepath.Join(authDir, "codex.json")
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}
	if err = os.WriteFile(authFile, data, 0o644); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}

	cfg := &config.Config{
		AuthDir: authDir,
		CodexKey: []config.CodexKey{
			{
				APIKey:  "x-key",
				BaseURL: "https://api.example.invalid/v1",
				Headers: map[string]string{"X-Req": "1"},
			},
		},
	}

	w := &Watcher{authDir: authDir}
	w.SetConfig(cfg)

	auths := w.SnapshotCoreAuths()
	if len(auths) != 2 {
		t.Fatalf("expected 2 auth entries (1 config + 1 file), got %d", len(auths))
	}

	var codexAPIKeyAuth *coreauth.Auth
	var codexFileAuth *coreauth.Auth
	for _, a := range auths {
		switch {
		case a.Provider == "codex" && a.Attributes["api_key"] == "x-key":
			codexAPIKeyAuth = a
		case a.Provider == "codex" && a.Attributes["api_key"] == "":
			codexFileAuth = a
		}
	}
	if codexAPIKeyAuth == nil {
		t.Fatal("expected synthesized Codex API key auth")
	}
	if got := codexAPIKeyAuth.Attributes["base_url"]; got == "" {
		t.Fatal("expected synthesized Codex API key auth to retain base_url")
	}
	if codexAPIKeyAuth.Attributes["auth_kind"] != "apikey" {
		t.Fatalf("expected auth_kind=apikey, got %s", codexAPIKeyAuth.Attributes["auth_kind"])
	}

	if codexFileAuth == nil {
		t.Fatal("expected codex auth from file")
	}
	if codexFileAuth.Attributes["auth_kind"] != "oauth" {
		t.Fatalf("expected auth_kind=oauth, got %s", codexFileAuth.Attributes["auth_kind"])
	}
}

func TestReloadConfigIfChanged_TriggersOnChangeAndSkipsUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.yaml")
	writeConfig := func(port int, allowRemote bool) {
		cfg := &config.Config{
			Port:    port,
			AuthDir: authDir,
			RemoteManagement: config.RemoteManagement{
				AllowRemote: allowRemote,
			},
		}
		data, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatalf("failed to marshal config: %v", err)
		}
		if err = os.WriteFile(configPath, data, 0o644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
	}

	writeConfig(8080, false)

	reloads := 0
	w := &Watcher{
		configPath:     configPath,
		authDir:        authDir,
		reloadCallback: func(*config.Config) { reloads++ },
	}

	w.reloadConfigIfChanged()
	if reloads != 1 {
		t.Fatalf("expected first reload to trigger callback once, got %d", reloads)
	}

	// Same content should be skipped by hash check.
	w.reloadConfigIfChanged()
	if reloads != 1 {
		t.Fatalf("expected unchanged config to be skipped, callback count %d", reloads)
	}

	writeConfig(9090, true)
	w.reloadConfigIfChanged()
	if reloads != 2 {
		t.Fatalf("expected changed config to trigger reload, callback count %d", reloads)
	}
	w.clientsMutex.RLock()
	defer w.clientsMutex.RUnlock()
	if w.config == nil || w.config.Port != 9090 || !w.config.RemoteManagement.AllowRemote {
		t.Fatalf("expected config to be updated after reload, got %+v", w.config)
	}
}

func TestStartAndStopSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("auth-dir: "+authDir), 0o644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	var reloads int32
	w, err := NewWatcher(configPath, authDir, func(*config.Config) {
		atomic.AddInt32(&reloads, 1)
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	w.SetConfig(&config.Config{AuthDir: authDir})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("expected Start to succeed: %v", err)
	}
	cancel()
	if err := w.Stop(); err != nil {
		t.Fatalf("expected Stop to succeed: %v", err)
	}
	if got := atomic.LoadInt32(&reloads); got != 1 {
		t.Fatalf("expected one reload callback, got %d", got)
	}
}

func TestDispatchRuntimeAuthUpdateEnqueuesAndUpdatesState(t *testing.T) {
	queue := make(chan AuthUpdate, 4)
	w := &Watcher{}
	w.SetAuthUpdateQueue(queue)
	defer w.stopDispatch()

	auth := &coreauth.Auth{ID: "auth-1", Provider: "test"}
	if ok := w.DispatchRuntimeAuthUpdate(AuthUpdate{Action: AuthUpdateActionAdd, Auth: auth}); !ok {
		t.Fatal("expected DispatchRuntimeAuthUpdate to enqueue")
	}

	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionAdd || update.Auth.ID != "auth-1" {
			t.Fatalf("unexpected update: %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for auth update")
	}

	if ok := w.DispatchRuntimeAuthUpdate(AuthUpdate{Action: AuthUpdateActionDelete, ID: "auth-1"}); !ok {
		t.Fatal("expected delete update to enqueue")
	}
	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionDelete || update.ID != "auth-1" {
			t.Fatalf("unexpected delete update: %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delete update")
	}
	w.clientsMutex.RLock()
	if _, exists := w.runtimeAuths["auth-1"]; exists {
		w.clientsMutex.RUnlock()
		t.Fatal("expected runtime auth to be cleared after delete")
	}
	w.clientsMutex.RUnlock()
}

func TestAddOrUpdateClientSkipsUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "sample.json")
	if err := os.WriteFile(authFile, []byte(`{"type":"demo"}`), 0o644); err != nil {
		t.Fatalf("failed to create auth file: %v", err)
	}
	data, _ := os.ReadFile(authFile)
	sum := sha256.Sum256(data)

	var reloads int32
	w := &Watcher{
		authDir:        tmpDir,
		lastAuthHashes: make(map[string]string),
		reloadCallback: func(*config.Config) {
			atomic.AddInt32(&reloads, 1)
		},
	}
	w.SetConfig(&config.Config{AuthDir: tmpDir})
	// Use normalizeAuthPath to match how addOrUpdateClient stores the key
	w.lastAuthHashes[w.normalizeAuthPath(authFile)] = hexString(sum[:])

	w.addOrUpdateClient(authFile)
	if got := atomic.LoadInt32(&reloads); got != 0 {
		t.Fatalf("expected no reload for unchanged file, got %d", got)
	}
}

func TestAddOrUpdateClientTriggersReloadAndHash(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "sample.json")
	if err := os.WriteFile(authFile, []byte(`{"type":"demo","api_key":"k"}`), 0o644); err != nil {
		t.Fatalf("failed to create auth file: %v", err)
	}

	var reloads int32
	w := &Watcher{
		authDir:        tmpDir,
		lastAuthHashes: make(map[string]string),
		reloadCallback: func(*config.Config) {
			atomic.AddInt32(&reloads, 1)
		},
	}
	w.SetConfig(&config.Config{AuthDir: tmpDir})

	w.addOrUpdateClient(authFile)

	if got := atomic.LoadInt32(&reloads); got != 0 {
		t.Fatalf("expected no reload callback for auth update, got %d", got)
	}
	// Use normalizeAuthPath to match how addOrUpdateClient stores the key
	normalized := w.normalizeAuthPath(authFile)
	if _, ok := w.lastAuthHashes[normalized]; !ok {
		t.Fatalf("expected hash to be stored for %s", normalized)
	}
}

func TestRemoveClientRemovesHash(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "sample.json")
	var reloads int32

	w := &Watcher{
		authDir:        tmpDir,
		lastAuthHashes: make(map[string]string),
		reloadCallback: func(*config.Config) {
			atomic.AddInt32(&reloads, 1)
		},
	}
	w.SetConfig(&config.Config{AuthDir: tmpDir})
	// Use normalizeAuthPath to set up the hash with the correct key format
	w.lastAuthHashes[w.normalizeAuthPath(authFile)] = "hash"

	w.removeClient(authFile)
	if _, ok := w.lastAuthHashes[w.normalizeAuthPath(authFile)]; ok {
		t.Fatal("expected hash to be removed after deletion")
	}
	if got := atomic.LoadInt32(&reloads); got != 0 {
		t.Fatalf("expected no reload callback for auth removal, got %d", got)
	}
}

func TestAuthFileEventsDoNotInvokeSnapshotCoreAuths(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "sample.json")
	if err := os.WriteFile(authFile, []byte(`{"type":"codex","email":"u@example.com"}`), 0o644); err != nil {
		t.Fatalf("failed to create auth file: %v", err)
	}

	origSnapshot := snapshotCoreAuthsFunc
	var snapshotCalls int32
	snapshotCoreAuthsFunc = func(cfg *config.Config, authDir string) []*coreauth.Auth {
		atomic.AddInt32(&snapshotCalls, 1)
		return origSnapshot(cfg, authDir)
	}
	defer func() { snapshotCoreAuthsFunc = origSnapshot }()

	w := &Watcher{
		authDir:          tmpDir,
		lastAuthHashes:   make(map[string]string),
		lastAuthContents: make(map[string]*coreauth.Auth),
		fileAuthsByPath:  make(map[string]map[string]*coreauth.Auth),
	}
	w.SetConfig(&config.Config{AuthDir: tmpDir})

	w.addOrUpdateClient(authFile)
	w.removeClient(authFile)

	if got := atomic.LoadInt32(&snapshotCalls); got != 0 {
		t.Fatalf("expected auth file events to avoid full snapshot, got %d calls", got)
	}
}

func TestAuthSliceToMap(t *testing.T) {
	t.Parallel()

	valid1 := &coreauth.Auth{ID: "a"}
	valid2 := &coreauth.Auth{ID: "b"}
	dupOld := &coreauth.Auth{ID: "dup", Label: "old"}
	dupNew := &coreauth.Auth{ID: "dup", Label: "new"}
	empty := &coreauth.Auth{ID: "  "}

	tests := []struct {
		name string
		in   []*coreauth.Auth
		want map[string]*coreauth.Auth
	}{
		{
			name: "nil input",
			in:   nil,
			want: map[string]*coreauth.Auth{},
		},
		{
			name: "empty input",
			in:   []*coreauth.Auth{},
			want: map[string]*coreauth.Auth{},
		},
		{
			name: "filters invalid auths",
			in:   []*coreauth.Auth{nil, empty},
			want: map[string]*coreauth.Auth{},
		},
		{
			name: "keeps valid auths",
			in:   []*coreauth.Auth{valid1, nil, valid2},
			want: map[string]*coreauth.Auth{"a": valid1, "b": valid2},
		},
		{
			name: "last duplicate wins",
			in:   []*coreauth.Auth{dupOld, dupNew},
			want: map[string]*coreauth.Auth{"dup": dupNew},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := authSliceToMap(tc.in)
			if len(tc.want) == 0 {
				if got == nil {
					t.Fatal("expected empty map, got nil")
				}
				if len(got) != 0 {
					t.Fatalf("expected empty map, got %#v", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("unexpected map length: got %d, want %d", len(got), len(tc.want))
			}
			for id, wantAuth := range tc.want {
				gotAuth, ok := got[id]
				if !ok {
					t.Fatalf("missing id %q in result map", id)
				}
				if !authEqual(gotAuth, wantAuth) {
					t.Fatalf("unexpected auth for id %q: got %#v, want %#v", id, gotAuth, wantAuth)
				}
			}
		})
	}
}

func TestTriggerServerUpdateCancelsPendingTimerOnImmediate(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{AuthDir: tmpDir}

	var reloads int32
	w := &Watcher{
		reloadCallback: func(*config.Config) {
			atomic.AddInt32(&reloads, 1)
		},
	}
	w.SetConfig(cfg)

	w.serverUpdateMu.Lock()
	w.serverUpdateLast = time.Now().Add(-(serverUpdateDebounce - 100*time.Millisecond))
	w.serverUpdateMu.Unlock()
	w.triggerServerUpdate(cfg)

	if got := atomic.LoadInt32(&reloads); got != 0 {
		t.Fatalf("expected no immediate reload, got %d", got)
	}

	w.serverUpdateMu.Lock()
	if !w.serverUpdatePend || w.serverUpdateTimer == nil {
		w.serverUpdateMu.Unlock()
		t.Fatal("expected a pending server update timer")
	}
	w.serverUpdateLast = time.Now().Add(-(serverUpdateDebounce + 10*time.Millisecond))
	w.serverUpdateMu.Unlock()

	w.triggerServerUpdate(cfg)
	if got := atomic.LoadInt32(&reloads); got != 1 {
		t.Fatalf("expected immediate reload once, got %d", got)
	}

	time.Sleep(250 * time.Millisecond)
	if got := atomic.LoadInt32(&reloads); got != 1 {
		t.Fatalf("expected pending timer to be cancelled, got %d reloads", got)
	}
}

func TestReloadClientsCachesAuthHashes(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "one.json")
	if err := os.WriteFile(authFile, []byte(`{"type":"demo"}`), 0o644); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}
	w := &Watcher{
		authDir: tmpDir,
		config:  &config.Config{AuthDir: tmpDir},
	}

	w.reloadClients(true, false)

	w.clientsMutex.RLock()
	defer w.clientsMutex.RUnlock()
	if len(w.lastAuthHashes) != 1 {
		t.Fatalf("expected hash cache for one auth file, got %d", len(w.lastAuthHashes))
	}
}

func TestReloadClientsLogsConfigDiffs(t *testing.T) {
	tmpDir := t.TempDir()
	oldCfg := &config.Config{AuthDir: tmpDir, Port: 1}
	newCfg := &config.Config{AuthDir: tmpDir, Port: 2}

	w := &Watcher{
		authDir: tmpDir,
		config:  oldCfg,
	}
	w.SetConfig(oldCfg)
	w.oldConfigYaml, _ = yaml.Marshal(oldCfg)

	w.clientsMutex.Lock()
	w.config = newCfg
	w.clientsMutex.Unlock()

	w.reloadClients(false, false)
}

func TestReloadClientsHandlesNilConfig(t *testing.T) {
	w := &Watcher{}
	w.reloadClients(true, false)
}

func TestSetAuthUpdateQueueNilResetsDispatch(t *testing.T) {
	w := &Watcher{}
	queue := make(chan AuthUpdate, 1)
	w.SetAuthUpdateQueue(queue)
	if w.dispatchCond == nil || w.dispatchCancel == nil {
		t.Fatal("expected dispatch to be initialized")
	}
	w.SetAuthUpdateQueue(nil)
	if w.dispatchCancel != nil {
		t.Fatal("expected dispatch cancel to be cleared when queue nil")
	}
}

func TestStopConfigReloadTimerSafeWhenNil(t *testing.T) {
	w := &Watcher{}
	w.stopConfigReloadTimer()
	w.configReloadMu.Lock()
	w.configReloadTimer = time.AfterFunc(10*time.Millisecond, func() {})
	w.configReloadMu.Unlock()
	time.Sleep(1 * time.Millisecond)
	w.stopConfigReloadTimer()
}

func TestDispatchAuthUpdatesFlushesQueue(t *testing.T) {
	queue := make(chan AuthUpdate, 4)
	w := &Watcher{}
	w.SetAuthUpdateQueue(queue)
	defer w.stopDispatch()

	w.dispatchAuthUpdates([]AuthUpdate{
		{Action: AuthUpdateActionAdd, ID: "a"},
		{Action: AuthUpdateActionModify, ID: "b"},
	})

	got := make([]AuthUpdate, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case u := <-queue:
			got = append(got, u)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for update %d", i)
		}
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected updates order/content: %+v", got)
	}
}

func TestDispatchLoopExitsOnContextDoneWhileSending(t *testing.T) {
	queue := make(chan AuthUpdate) // unbuffered to block sends
	w := &Watcher{
		authQueue: queue,
		pendingUpdates: map[string]AuthUpdate{
			"k": {Action: AuthUpdateActionAdd, ID: "k"},
		},
		pendingOrder: []string{"k"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.dispatchLoop(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected dispatchLoop to exit after ctx canceled while blocked on send")
	}
}

func TestRefreshAuthStateDispatchesRuntimeAuths(t *testing.T) {
	queue := make(chan AuthUpdate, 8)
	w := &Watcher{
		authDir:        t.TempDir(),
		lastAuthHashes: make(map[string]string),
	}
	w.SetConfig(&config.Config{AuthDir: w.authDir})
	w.SetAuthUpdateQueue(queue)
	defer w.stopDispatch()

	w.clientsMutex.Lock()
	w.runtimeAuths = map[string]*coreauth.Auth{
		"nil": nil,
		"r1":  {ID: "r1", Provider: "runtime"},
	}
	w.clientsMutex.Unlock()

	w.refreshAuthState(false)

	select {
	case u := <-queue:
		if u.Action != AuthUpdateActionAdd || u.ID != "r1" {
			t.Fatalf("unexpected auth update: %+v", u)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime auth update")
	}
}

func TestReloadAuthsFromStoreDispatchesStoreAuths(t *testing.T) {
	queue := make(chan AuthUpdate, 8)
	w := &Watcher{
		authDir:        t.TempDir(),
		lastAuthHashes: make(map[string]string),
	}
	w.SetConfig(&config.Config{AuthDir: w.authDir})
	w.SetAuthUpdateQueue(queue)
	defer w.stopDispatch()

	storeAuth := &coreauth.Auth{
		ID:       "nacos-auth-1",
		Provider: "codex",
		Label:    "real@example.com",
		Metadata: map[string]any{"type": "codex", "email": "real@example.com"},
	}

	w.reloadAuthsFromStore([]*coreauth.Auth{storeAuth})

	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionAdd {
			t.Fatalf("expected add update, got %+v", update)
		}
		if update.ID != storeAuth.ID {
			t.Fatalf("expected update for %q, got %+v", storeAuth.ID, update)
		}
		if update.Auth == nil || update.Auth.Label != storeAuth.Label {
			t.Fatalf("expected store auth payload to be dispatched, got %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for store auth update")
	}
}

type startTestAuthStore struct {
	auths []*coreauth.Auth
}

func (s *startTestAuthStore) List(context.Context) ([]*coreauth.Auth, error) {
	return s.auths, nil
}

func (s *startTestAuthStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}

func (s *startTestAuthStore) Delete(context.Context, string) error {
	return nil
}

func (s *startTestAuthStore) ReadByName(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (s *startTestAuthStore) ListMetadata(context.Context) ([]nacos.AuthFileMetadata, error) {
	return nil, nil
}

func (s *startTestAuthStore) Watch(context.Context, func([]*coreauth.Auth)) error {
	return nil
}

func (s *startTestAuthStore) StopWatch() {}

func TestStartSeedsStoreAuthsBeforeWatchingChanges(t *testing.T) {
	queue := make(chan AuthUpdate, 4)
	storeAuth := &coreauth.Auth{ID: "real-codex", Provider: "codex", Label: "real@example.com"}
	store := &startTestAuthStore{auths: []*coreauth.Auth{storeAuth}}

	w, err := NewWatcher(filepath.Join(t.TempDir(), "config.yaml"), t.TempDir(), nil, nil, store)
	if err != nil {
		t.Fatalf("NewWatcher error = %v", err)
	}
	w.SetConfig(&config.Config{AuthDir: w.authDir})
	w.SetAuthUpdateQueue(queue)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer w.Stop()

	if err = w.Start(ctx); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	select {
	case update := <-queue:
		if update.Action != AuthUpdateActionAdd || update.ID != storeAuth.ID {
			t.Fatalf("expected initial add for seeded store auth, got %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for seeded store auth update")
	}
}

func TestRefreshAuthStateDeletesReplacedStoreAuths(t *testing.T) {
	queue := make(chan AuthUpdate, 8)
	w := &Watcher{
		authDir:        t.TempDir(),
		lastAuthHashes: make(map[string]string),
		currentAuths: map[string]*coreauth.Auth{
			"real-codex": {ID: "real-codex", Provider: "codex", Label: "real@example.com"},
		},
	}
	w.SetConfig(&config.Config{AuthDir: w.authDir})
	w.SetAuthUpdateQueue(queue)
	defer w.stopDispatch()

	w.clientsMutex.Lock()
	w.storeAuths = map[string]*coreauth.Auth{
		"alpha.json": {ID: "alpha.json", Provider: "codex", Label: "alpha@example.com"},
		"beta.json":  {ID: "beta.json", Provider: "codex", Label: "beta@example.com", Disabled: true, Status: coreauth.StatusDisabled},
	}
	w.clientsMutex.Unlock()

	w.refreshAuthState(false)

	seen := map[string]AuthUpdateAction{}
	deadline := time.After(2 * time.Second)
	for len(seen) < 3 {
		select {
		case update := <-queue:
			seen[update.ID] = update.Action
		case <-deadline:
			t.Fatalf("timed out waiting for auth replacement updates, got %+v", seen)
		}
	}

	if seen["real-codex"] != AuthUpdateActionDelete {
		t.Fatalf("expected delete update for real-codex, got %+v", seen)
	}
	if seen["alpha.json"] != AuthUpdateActionAdd {
		t.Fatalf("expected add update for alpha.json, got %+v", seen)
	}
	if seen["beta.json"] != AuthUpdateActionAdd {
		t.Fatalf("expected add update for beta.json, got %+v", seen)
	}
}

func TestAddOrUpdateClientEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := tmpDir
	authFile := filepath.Join(tmpDir, "edge.json")
	if err := os.WriteFile(authFile, []byte(`{"type":"demo"}`), 0o644); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}
	emptyFile := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(emptyFile, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to write empty auth file: %v", err)
	}

	var reloads int32
	w := &Watcher{
		authDir:        authDir,
		lastAuthHashes: make(map[string]string),
		reloadCallback: func(*config.Config) { atomic.AddInt32(&reloads, 1) },
	}

	w.addOrUpdateClient(filepath.Join(tmpDir, "missing.json"))
	w.addOrUpdateClient(emptyFile)
	if atomic.LoadInt32(&reloads) != 0 {
		t.Fatalf("expected no reloads for missing/empty file, got %d", reloads)
	}

	w.addOrUpdateClient(authFile) // config nil -> should not panic or update
	if len(w.lastAuthHashes) != 0 {
		t.Fatalf("expected no hash entries without config, got %d", len(w.lastAuthHashes))
	}
}

func TestLoadFileClientsWalkError(t *testing.T) {
	tmpDir := t.TempDir()
	noAccessDir := filepath.Join(tmpDir, "0noaccess")
	if err := os.MkdirAll(noAccessDir, 0o755); err != nil {
		t.Fatalf("failed to create noaccess dir: %v", err)
	}
	if err := os.Chmod(noAccessDir, 0); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	defer func() { _ = os.Chmod(noAccessDir, 0o755) }()

	cfg := &config.Config{AuthDir: tmpDir}
	w := &Watcher{}
	w.SetConfig(cfg)

	count := w.loadFileClients(cfg)
	if count != 0 {
		t.Fatalf("expected count 0 due to walk error, got %d", count)
	}
}

func TestReloadConfigIfChangedHandlesMissingAndEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	w := &Watcher{
		configPath: filepath.Join(tmpDir, "missing.yaml"),
		authDir:    authDir,
	}
	w.reloadConfigIfChanged() // missing file -> log + return

	emptyPath := filepath.Join(tmpDir, "empty.yaml")
	if err := os.WriteFile(emptyPath, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to write empty config: %v", err)
	}
	w.configPath = emptyPath
	w.reloadConfigIfChanged() // empty file -> early return
}

func TestReloadConfigFromSourceSkipsNoopConfigUpdate(t *testing.T) {
	authDir := t.TempDir()
	baseCfg := &config.Config{
		AuthDir:             authDir,
		Port:                8080,
		RequestRetry:        2,
		MaxRetryCredentials: 1,
		MaxRetryInterval:    30,
	}

	reloads := 0
	w := &Watcher{
		authDir:        authDir,
		lastAuthHashes: make(map[string]string),
		reloadCallback: func(*config.Config) { reloads++ },
	}
	w.SetConfig(baseCfg)

	clone := *baseCfg
	w.reloadConfigFromSource(&clone)

	if reloads != 0 {
		t.Fatalf("expected no reload callback for unchanged effective config, got %d", reloads)
	}
}

func TestReloadConfigTriggersCallbackForMaxRetryCredentialsChange(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}
	configPath := filepath.Join(tmpDir, "config.yaml")

	oldCfg := &config.Config{
		AuthDir:             authDir,
		MaxRetryCredentials: 0,
		RequestRetry:        1,
		MaxRetryInterval:    5,
	}
	newCfg := &config.Config{
		AuthDir:             authDir,
		MaxRetryCredentials: 2,
		RequestRetry:        1,
		MaxRetryInterval:    5,
	}
	data, errMarshal := yaml.Marshal(newCfg)
	if errMarshal != nil {
		t.Fatalf("failed to marshal config: %v", errMarshal)
	}
	if errWrite := os.WriteFile(configPath, data, 0o644); errWrite != nil {
		t.Fatalf("failed to write config: %v", errWrite)
	}

	callbackCalls := 0
	callbackMaxRetryCredentials := -1
	w := &Watcher{
		configPath:     configPath,
		authDir:        authDir,
		lastAuthHashes: make(map[string]string),
		reloadCallback: func(cfg *config.Config) {
			callbackCalls++
			if cfg != nil {
				callbackMaxRetryCredentials = cfg.MaxRetryCredentials
			}
		},
	}
	w.SetConfig(oldCfg)

	if ok := w.reloadConfig(); !ok {
		t.Fatal("expected reloadConfig to succeed")
	}

	if callbackCalls != 1 {
		t.Fatalf("expected reload callback to be called once, got %d", callbackCalls)
	}
	if callbackMaxRetryCredentials != 2 {
		t.Fatalf("expected callback MaxRetryCredentials=2, got %d", callbackMaxRetryCredentials)
	}

	w.clientsMutex.RLock()
	defer w.clientsMutex.RUnlock()
	if w.config == nil || w.config.MaxRetryCredentials != 2 {
		t.Fatalf("expected watcher config MaxRetryCredentials=2, got %+v", w.config)
	}
}

func TestDispatchRuntimeAuthUpdateReturnsFalseWithoutQueue(t *testing.T) {
	w := &Watcher{}
	if ok := w.DispatchRuntimeAuthUpdate(AuthUpdate{Action: AuthUpdateActionAdd, Auth: &coreauth.Auth{ID: "a"}}); ok {
		t.Fatal("expected DispatchRuntimeAuthUpdate to return false when no queue configured")
	}
	if ok := w.DispatchRuntimeAuthUpdate(AuthUpdate{Action: AuthUpdateActionDelete, Auth: &coreauth.Auth{ID: "a"}}); ok {
		t.Fatal("expected DispatchRuntimeAuthUpdate delete to return false when no queue configured")
	}
}

func TestNormalizeAuthNil(t *testing.T) {
	if normalizeAuth(nil) != nil {
		t.Fatal("expected normalizeAuth(nil) to return nil")
	}
}

type stubStore struct {
}

func (s *stubStore) List(context.Context) ([]*coreauth.Auth, error) { return nil, nil }
func (s *stubStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}
func (s *stubStore) Delete(context.Context, string) error { return nil }

func TestScheduleConfigReloadDebounces(t *testing.T) {
	tmp := t.TempDir()
	authDir := tmp
	cfgPath := tmp + "/config.yaml"
	if err := os.WriteFile(cfgPath, []byte("auth-dir: "+authDir+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var reloads int32
	w := &Watcher{
		configPath:     cfgPath,
		authDir:        authDir,
		reloadCallback: func(*config.Config) { atomic.AddInt32(&reloads, 1) },
	}
	w.SetConfig(&config.Config{AuthDir: authDir})

	w.scheduleConfigReload()
	w.scheduleConfigReload()

	time.Sleep(400 * time.Millisecond)

	if atomic.LoadInt32(&reloads) != 1 {
		t.Fatalf("expected single debounced reload, got %d", reloads)
	}
	if w.lastConfigHash == "" {
		t.Fatal("expected lastConfigHash to be set after reload")
	}
}

func TestPrepareAuthUpdatesLockedForceAndDelete(t *testing.T) {
	w := &Watcher{
		currentAuths: map[string]*coreauth.Auth{
			"a": {ID: "a", Provider: "p1"},
		},
		authQueue: make(chan AuthUpdate, 4),
	}

	updates := w.prepareAuthUpdatesLocked([]*coreauth.Auth{{ID: "a", Provider: "p2"}}, false)
	if len(updates) != 1 || updates[0].Action != AuthUpdateActionModify || updates[0].ID != "a" {
		t.Fatalf("unexpected modify updates: %+v", updates)
	}

	updates = w.prepareAuthUpdatesLocked([]*coreauth.Auth{{ID: "a", Provider: "p2"}}, true)
	if len(updates) != 1 || updates[0].Action != AuthUpdateActionModify {
		t.Fatalf("expected force modify, got %+v", updates)
	}

	updates = w.prepareAuthUpdatesLocked([]*coreauth.Auth{}, false)
	if len(updates) != 1 || updates[0].Action != AuthUpdateActionDelete || updates[0].ID != "a" {
		t.Fatalf("expected delete for missing auth, got %+v", updates)
	}
}

func TestAuthEqualIgnoresTemporalFields(t *testing.T) {
	now := time.Now()
	a := &coreauth.Auth{ID: "x", CreatedAt: now}
	b := &coreauth.Auth{ID: "x", CreatedAt: now.Add(5 * time.Second)}
	if !authEqual(a, b) {
		t.Fatal("expected authEqual to ignore temporal differences")
	}
}

func TestDispatchLoopExitsWhenQueueNilAndContextCanceled(t *testing.T) {
	w := &Watcher{
		dispatchCond:   nil,
		pendingUpdates: map[string]AuthUpdate{"k": {ID: "k"}},
		pendingOrder:   []string{"k"},
	}
	w.dispatchCond = sync.NewCond(&w.dispatchMu)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.dispatchLoop(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	w.dispatchMu.Lock()
	w.dispatchCond.Broadcast()
	w.dispatchMu.Unlock()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("dispatchLoop did not exit after context cancel")
	}
}

func TestReloadClientsFiltersOAuthProvidersWithoutRescan(t *testing.T) {
	tmp := t.TempDir()
	w := &Watcher{
		authDir: tmp,
		config:  &config.Config{AuthDir: tmp},
		currentAuths: map[string]*coreauth.Auth{
			"a": {ID: "a", Provider: "Match"},
			"b": {ID: "b", Provider: "other"},
		},
		lastAuthHashes: map[string]string{"cached": "hash"},
	}

	w.reloadClients(false, false)

	w.clientsMutex.RLock()
	defer w.clientsMutex.RUnlock()
	if len(w.lastAuthHashes) != 1 {
		t.Fatalf("expected existing hash cache to be retained, got %d", len(w.lastAuthHashes))
	}
}

func hexString(data []byte) string {
	return strings.ToLower(fmt.Sprintf("%x", data))
}
