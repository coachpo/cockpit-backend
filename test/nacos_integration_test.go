package test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	internalcmd "github.com/coachpo/cockpit-backend/internal/cmd"
	"github.com/coachpo/cockpit-backend/internal/logging"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"github.com/coachpo/cockpit-backend/internal/util"
	log "github.com/sirupsen/logrus"
)

const (
	nacosSmokeConfigDataID = "proxy-config"
	nacosSmokeAuthDataID   = "auth-credentials"
)

type nacosSmokeConfig struct {
	addr        string
	baseURL     string
	namespace   string
	group       string
	username    string
	password    string
	port        int
	authPayload string
}

type nacosDataSnapshot struct {
	exists  bool
	content string
}

type syncLogBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

type managementAuthFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
	Email    string `json:"email"`
}

type managementAuthFilesResponse struct {
	Files []managementAuthFile `json:"files"`
}

func (b *syncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (b *syncLogBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Len()
}

func TestNacosLiveSmoke(t *testing.T) {
	if !nacosSmokeEnabled() {
		t.Skip("set COCKPIT_NACOS_SMOKE=1 to run the live Nacos smoke test")
	}

	cfg := nacosSmokeConfigFromEnv(t)
	if err := runNacosSmokeTest(t, cfg); err != nil {
		t.Fatal(err)
	}
}

func nacosSmokeEnabled() bool {
	value := strings.TrimSpace(os.Getenv("COCKPIT_NACOS_SMOKE"))
	return value == "1" || strings.EqualFold(value, "true")
}

func nacosSmokeConfigFromEnv(t *testing.T) nacosSmokeConfig {
	t.Helper()

	addr := strings.TrimSpace(os.Getenv("NACOS_ADDR"))
	if addr == "" {
		t.Fatal("NACOS_ADDR is required")
	}
	username := strings.TrimSpace(os.Getenv("NACOS_USERNAME"))
	if username == "" {
		t.Fatal("NACOS_USERNAME is required")
	}
	password := os.Getenv("NACOS_PASSWORD")
	if password == "" {
		t.Fatal("NACOS_PASSWORD is required")
	}

	baseURL := addr
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	namespace := strings.TrimSpace(os.Getenv("NACOS_NAMESPACE"))
	if namespace == "" {
		namespace = "public"
	}
	group := strings.TrimSpace(os.Getenv("NACOS_GROUP"))
	if group == "" {
		group = "DEFAULT_GROUP"
	}

	return nacosSmokeConfig{
		addr:        addr,
		baseURL:     baseURL,
		namespace:   namespace,
		group:       group,
		username:    username,
		password:    password,
		port:        reserveLocalPort(t),
		authPayload: loadSmokeAuthPayload(t),
	}
}

func reserveLocalPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("reserve local port: unexpected address type %T", listener.Addr())
	}
	return addr.Port
}

func loadSmokeAuthPayload(t *testing.T) string {
	t.Helper()

	if path := strings.TrimSpace(os.Getenv("COCKPIT_NACOS_AUTH_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read COCKPIT_NACOS_AUTH_FILE %q: %v", path, err)
		}
		return normalizeAuthPayload(t, string(data), path)
	}

	if raw := strings.TrimSpace(os.Getenv("COCKPIT_NACOS_AUTH_JSON")); raw != "" {
		return normalizeAuthPayload(t, raw, "COCKPIT_NACOS_AUTH_JSON")
	}

	return `{"updated-codex":{"type":"codex","email":"updated-nacos-smoke@example.com","disabled":false}}`
}

func sampleSmokeAuthPayload() string {
	return `{"example-codex":{"type":"codex","email":"nacos-smoke@example.com","disabled":false}}`
}

func normalizeAuthPayload(t *testing.T, raw string, source string) string {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse auth payload from %s: %v", source, err)
	}
	if payload == nil {
		payload = map[string]any{}
	}

	if _, hasType := payload["type"]; hasType {
		authID := strings.TrimSpace(os.Getenv("COCKPIT_NACOS_AUTH_ID"))
		if authID == "" {
			authID = strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
		}
		if authID == "" || authID == "." || authID == "COCKPIT_NACOS_AUTH_JSON" {
			authID = "imported-auth"
		}
		wrapped := compactAuthEntriesForNacos(t, map[string]any{authID: payload})
		return canonicalJSON(t, wrapped)
	}

	for key, value := range payload {
		if _, ok := value.(map[string]any); !ok {
			t.Fatalf("auth payload from %s must be a single record or a map of auth records; key %q is %T", source, key, value)
		}
	}

	return canonicalJSON(t, compactAuthEntriesForNacos(t, payload))
}

func compactAuthEntriesForNacos(t *testing.T, entries map[string]any) map[string]any {
	t.Helper()
	if len(entries) != 1 {
		return entries
	}

	var (
		entryID  string
		entryMap map[string]any
	)
	for id, raw := range entries {
		entryID = id
		mapped, ok := raw.(map[string]any)
		if !ok {
			return entries
		}
		entryMap = mapped
	}
	if strings.TrimSpace(strings.ToLower(stringValueAny(entryMap, "type"))) != "codex" {
		return entries
	}
	if len(entryMap) == 0 || stringValueAny(entryMap, "access_token") == "" {
		return entries
	}

	canonical := func(value map[string]any) string {
		return canonicalJSON(t, value)
	}
	if len(canonical(entries)) <= 3000 {
		return entries
	}

	compact := func(keys []string) map[string]any {
		cloned := make(map[string]any)
		for _, key := range keys {
			if value, ok := entryMap[key]; ok {
				cloned[key] = value
			}
		}
		if _, ok := cloned["type"]; !ok {
			cloned["type"] = "codex"
		}
		if _, ok := cloned["disabled"]; !ok {
			cloned["disabled"] = false
		}
		return map[string]any{entryID: cloned}
	}

	withIDToken := compact([]string{"access_token", "refresh_token", "id_token", "account_id", "email", "type", "disabled", "websocket"})
	if len(canonical(withIDToken)) <= 3000 {
		return withIDToken
	}
	return compact([]string{"access_token", "refresh_token", "account_id", "email", "type", "disabled", "websocket"})
}

func stringValueAny(values map[string]any, key string) string {
	if len(values) == 0 || key == "" {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func canonicalJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal canonical JSON: %v", err)
	}
	return string(data)
}

func canonicalJSONString(t *testing.T, raw string) string {
	t.Helper()

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("parse JSON string %q: %v", raw, err)
	}
	return canonicalJSON(t, value)
}

func expectedAuthFile(payload string) (managementAuthFile, error) {
	var entries map[string]map[string]any
	if err := json.Unmarshal([]byte(payload), &entries); err != nil {
		return managementAuthFile{}, fmt.Errorf("parse auth entries %q: %w", payload, err)
	}
	if len(entries) == 0 {
		return managementAuthFile{}, fmt.Errorf("expected auth payload to contain at least one entry")
	}

	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	id := ids[0]
	entry := entries[id]
	name, _ := entry["file_name"].(string)
	if strings.TrimSpace(name) == "" {
		name = id + ".json"
	}
	email, _ := entry["email"].(string)
	provider, _ := entry["type"].(string)
	return managementAuthFile{ID: id, Name: name, Email: email, Type: provider, Provider: provider}, nil
}

func firstAuthRecordJSON(payload string) (string, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &entries); err != nil {
		return "", fmt.Errorf("parse auth payload %q: %w", payload, err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("expected auth payload to contain at least one entry")
	}
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return string(entries[ids[0]]), nil
}

func runNacosSmokeTest(t *testing.T, cfg nacosSmokeConfig) error {
	t.Helper()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	token, err := nacosLogin(httpClient, cfg)
	if err != nil {
		return err
	}

	configSnapshot, err := nacosFetchSnapshot(httpClient, cfg, token, nacosSmokeConfigDataID)
	if err != nil {
		return err
	}
	authSnapshot, err := nacosFetchSnapshot(httpClient, cfg, token, nacosSmokeAuthDataID)
	if err != nil {
		return err
	}
	t.Cleanup(func() {
		if errRestore := restoreNacosSnapshot(httpClient, cfg, token, nacosSmokeConfigDataID, "yaml", configSnapshot); errRestore != nil {
			t.Errorf("restore %s: %v", nacosSmokeConfigDataID, errRestore)
		}
		if errRestore := restoreNacosSnapshot(httpClient, cfg, token, nacosSmokeAuthDataID, "json", authSnapshot); errRestore != nil {
			t.Errorf("restore %s: %v", nacosSmokeAuthDataID, errRestore)
		}
	})

	if err = nacosPublishConfig(httpClient, cfg, token, nacosSmokeConfigDataID, buildSmokeConfig(cfg.port, false, false), "yaml"); err != nil {
		return err
	}
	if err = nacosPublishConfig(httpClient, cfg, token, nacosSmokeAuthDataID, sampleSmokeAuthPayload(), "json"); err != nil {
		return err
	}

	t.Setenv("NACOS_ADDR", cfg.addr)
	t.Setenv("NACOS_NAMESPACE", cfg.namespace)
	t.Setenv("NACOS_GROUP", cfg.group)
	t.Setenv("NACOS_USERNAME", cfg.username)
	t.Setenv("NACOS_PASSWORD", cfg.password)
	runtimeClient, err := nacos.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("create runtime nacos client: %w", err)
	}
	if runtimeClient == nil {
		return fmt.Errorf("runtime nacos client was nil")
	}

	configSource := nacos.NewNacosConfigStore(runtimeClient)
	authStore := nacos.NewNacosAuthStore(runtimeClient)
	loadedCfg, err := configSource.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config from nacos: %w", err)
	}
	resolvedAuthDir, err := util.ResolveAuthDir(loadedCfg.AuthDir)
	if err != nil {
		return fmt.Errorf("resolve auth dir: %w", err)
	}
	loadedCfg.AuthDir = resolvedAuthDir

	logging.SetupBaseLogger()
	if err = logging.ConfigureLogOutput(loadedCfg); err != nil {
		return fmt.Errorf("configure log output: %w", err)
	}

	buffer := &syncLogBuffer{}
	previousOutput := log.StandardLogger().Out
	previousLevel := log.GetLevel()
	log.SetOutput(buffer)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetLevel(previousLevel)
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cancel, done := internalcmd.StartServiceBackground(loadedCfg, configPath, configSource, authStore)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("timed out waiting for smoke-test service shutdown")
		}
	})

	if err = waitForHTTPStatus(cfg.port, http.StatusOK, 10*time.Second); err != nil {
		return err
	}
	if err = waitForManagementAuth(httpClient, cfg.port, sampleSmokeAuthPayload(), 10*time.Second); err != nil {
		return err
	}

	updateOffset := buffer.Len()
	if err = nacosPublishConfig(httpClient, cfg, token, nacosSmokeConfigDataID, buildSmokeConfig(cfg.port, true, true), "yaml"); err != nil {
		return err
	}
	if err = waitForBufferContainsAfter(buffer, updateOffset, "config reloaded from source, triggering client reload", 10*time.Second); err != nil {
		return err
	}

	requestOffset := buffer.Len()
	if err = sendLocalRequest(cfg.port); err != nil {
		return err
	}
	if err = waitForBufferContainsAfter(buffer, requestOffset, `GET     "/"`, 5*time.Second); err != nil {
		return err
	}

	replacement, err := firstAuthRecordJSON(cfg.authPayload)
	if err != nil {
		return err
	}
	wantAuth, err := expectedAuthFile(cfg.authPayload)
	if err != nil {
		return err
	}
	if err = uploadManagementAuthFile(httpClient, cfg.port, wantAuth.Name, replacement); err != nil {
		return err
	}

	if err = waitForManagementAuth(httpClient, cfg.port, cfg.authPayload, 10*time.Second); err != nil {
		return err
	}

	return nil
}

func TestBuildSmokeConfig_UsesRetainedSchema(t *testing.T) {
	const want = `host: ""
port: 8317

auth-dir: "~/.cockpit"

disable-cooling: true
request-retry: 3
max-retry-credentials: 0
max-retry-interval: 30
passthrough-headers: true

quota-exceeded:
  switch-project: true

routing:
  strategy: "round-robin"

ws-auth: false
`

	if got := buildSmokeConfig(8317, true, true); got != want {
		t.Fatalf("unexpected smoke config:\n%s", got)
	}
}

func buildSmokeConfig(port int, disableCooling bool, passthroughHeaders bool) string {
	return fmt.Sprintf(`host: ""
port: %d

auth-dir: "~/.cockpit"

disable-cooling: %t
request-retry: 3
max-retry-credentials: 0
max-retry-interval: 30
passthrough-headers: %t

quota-exceeded:
  switch-project: true

routing:
  strategy: "round-robin"

ws-auth: false
`, port, disableCooling, passthroughHeaders)
}

func nacosLogin(httpClient *http.Client, cfg nacosSmokeConfig) (string, error) {
	form := url.Values{}
	form.Set("username", cfg.username)
	form.Set("password", cfg.password)

	resp, err := httpClient.PostForm(cfg.baseURL+"/nacos/v1/auth/login", form)
	if err != nil {
		return "", fmt.Errorf("nacos login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("nacos login returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		AccessToken string `json:"accessToken"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode nacos login response: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("nacos login returned empty access token")
	}
	return payload.AccessToken, nil
}

func nacosPublishConfig(httpClient *http.Client, cfg nacosSmokeConfig, token string, dataID string, content string, dataType string) error {
	form := url.Values{}
	form.Set("dataId", dataID)
	form.Set("group", cfg.group)
	form.Set("content", content)
	form.Set("type", dataType)
	if includeTenant(cfg.namespace) {
		form.Set("tenant", cfg.namespace)
	}

	requestURL := cfg.baseURL + "/nacos/v1/cs/configs?accessToken=" + url.QueryEscape(token)
	resp, err := httpClient.PostForm(requestURL, form)
	if err != nil {
		return fmt.Errorf("publish %s: %w", dataID, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("publish %s returned %d: %s", dataID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(string(body)) != "true" {
		return fmt.Errorf("publish %s returned unexpected body %q", dataID, strings.TrimSpace(string(body)))
	}
	return nil
}

func nacosFetchSnapshot(httpClient *http.Client, cfg nacosSmokeConfig, token string, dataID string) (nacosDataSnapshot, error) {
	content, exists, err := nacosFetchConfig(httpClient, cfg, token, dataID)
	if err != nil {
		return nacosDataSnapshot{}, err
	}
	return nacosDataSnapshot{exists: exists, content: content}, nil
}

func nacosFetchRequired(httpClient *http.Client, cfg nacosSmokeConfig, token string, dataID string) (string, error) {
	content, exists, err := nacosFetchConfig(httpClient, cfg, token, dataID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("expected %s to exist in nacos", dataID)
	}
	return content, nil
}

func nacosFetchConfig(httpClient *http.Client, cfg nacosSmokeConfig, token string, dataID string) (string, bool, error) {
	query := url.Values{}
	query.Set("accessToken", token)
	query.Set("dataId", dataID)
	query.Set("group", cfg.group)
	if includeTenant(cfg.namespace) {
		query.Set("tenant", cfg.namespace)
	}

	requestURL := cfg.baseURL + "/nacos/v1/cs/configs?" + query.Encode()
	resp, err := httpClient.Get(requestURL)
	if err != nil {
		return "", false, fmt.Errorf("fetch %s: %w", dataID, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("fetch %s returned %d: %s", dataID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body), true, nil
}

func restoreNacosSnapshot(httpClient *http.Client, cfg nacosSmokeConfig, token string, dataID string, dataType string, snapshot nacosDataSnapshot) error {
	if snapshot.exists {
		return nacosPublishConfig(httpClient, cfg, token, dataID, snapshot.content, dataType)
	}
	return nacosDeleteConfig(httpClient, cfg, token, dataID)
}

func nacosDeleteConfig(httpClient *http.Client, cfg nacosSmokeConfig, token string, dataID string) error {
	query := url.Values{}
	query.Set("accessToken", token)
	query.Set("dataId", dataID)
	query.Set("group", cfg.group)
	if includeTenant(cfg.namespace) {
		query.Set("tenant", cfg.namespace)
	}

	req, err := http.NewRequest(http.MethodDelete, cfg.baseURL+"/nacos/v1/cs/configs?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("build delete request for %s: %w", dataID, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete %s: %w", dataID, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete %s returned %d: %s", dataID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(string(body)) != "true" {
		return fmt.Errorf("delete %s returned unexpected body %q", dataID, strings.TrimSpace(string(body)))
	}
	return nil
}

func includeTenant(namespace string) bool {
	namespace = strings.TrimSpace(namespace)
	return namespace != "" && !strings.EqualFold(namespace, "public")
}

func waitForHTTPStatus(port int, expected int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == expected {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for HTTP %d on port %d", expected, port)
}

func sendLocalRequest(port int) error {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return fmt.Errorf("send local request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("local request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func waitForManagementAuth(httpClient *http.Client, port int, payload string, timeout time.Duration) error {
	want, err := expectedAuthFile(payload)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		files, errFetch := fetchManagementAuthFiles(httpClient, port)
		if errFetch == nil {
			for _, got := range files {
				if (got.ID == want.ID || got.ID == want.Name) && got.Email == want.Email && strings.EqualFold(got.Type, want.Type) {
					return nil
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}

	files, err := fetchManagementAuthFiles(httpClient, port)
	if err != nil {
		return err
	}
	return fmt.Errorf("timed out waiting for management auth state %s, got %+v", payload, files)
}

func fetchManagementAuthFiles(httpClient *http.Client, port int) ([]managementAuthFile, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/auth-files", port), nil)
	if err != nil {
		return nil, fmt.Errorf("build management auth request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch management auth files: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("management auth files returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload managementAuthFilesResponse
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode management auth files response: %w", err)
	}
	return payload.Files, nil
}

func uploadManagementAuthFile(httpClient *http.Client, port int, name string, body string) error {
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/api/auth-files?name=%s", port, url.QueryEscape(name)),
		strings.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build management upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload management auth file: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("management auth upload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func waitForBufferContainsAfter(buffer *syncLogBuffer, offset int, needle string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		content := buffer.String()
		if offset < 0 {
			offset = 0
		}
		if offset > len(content) {
			offset = len(content)
		}
		if strings.Contains(content[offset:], needle) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	content := buffer.String()
	if offset < 0 {
		offset = 0
	}
	if offset > len(content) {
		offset = len(content)
	}
	return fmt.Errorf("timed out waiting for log containing %q after offset %d; observed logs: %s", needle, offset, strings.TrimSpace(content[offset:]))
}
