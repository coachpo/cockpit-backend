package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/gin-gonic/gin"
)

func TestGetRuntimeSettings_ReturnsAggregatedResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutPersistence(&config.Config{
		WebsocketAuth:    true,
		RequestRetry:     3,
		MaxRetryInterval: 45,
		Routing:          config.RoutingConfig{Strategy: "fill-first"},
		QuotaExceeded:    config.QuotaExceeded{SwitchProject: true},
	}, nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/runtime-settings", nil)

	h.GetRuntimeSettings(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		WebsocketAuth    bool   `json:"ws-auth"`
		RequestRetry     int    `json:"request-retry"`
		MaxRetryInterval int    `json:"max-retry-interval"`
		RoutingStrategy  string `json:"routing-strategy"`
		SwitchProject    bool   `json:"switch-project"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.WebsocketAuth || response.RequestRetry != 3 || response.MaxRetryInterval != 45 || response.RoutingStrategy != "fill-first" || !response.SwitchProject {
		t.Fatalf("unexpected runtime settings response: %#v", response)
	}
}

func TestPutRuntimeSettings_UpdatesAllFieldsAndPersists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutPersistence(&config.Config{
		Routing:       config.RoutingConfig{Strategy: "round-robin"},
		QuotaExceeded: config.QuotaExceeded{},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/runtime-settings", bytes.NewBufferString(`{"ws-auth":true,"request-retry":4,"max-retry-interval":90,"routing-strategy":"fill-first","switch-project":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutRuntimeSettings(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !h.cfg.WebsocketAuth || h.cfg.RequestRetry != 4 || h.cfg.MaxRetryInterval != 90 || h.cfg.Routing.Strategy != "fill-first" || !h.cfg.QuotaExceeded.SwitchProject {
		t.Fatalf("expected config to be updated, got %#v", h.cfg)
	}
}

func TestPutRuntimeSettings_LegacyStrategyRejectedWithoutMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutPersistence(&config.Config{
		RequestRetry:     1,
		MaxRetryInterval: 30,
		Routing:          config.RoutingConfig{Strategy: "round-robin"},
		QuotaExceeded:    config.QuotaExceeded{},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/runtime-settings", bytes.NewBufferString(`{"ws-auth":true,"request-retry":4,"max-retry-interval":90,"routing-strategy":"fillfirst","switch-project":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutRuntimeSettings(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if h.cfg.WebsocketAuth || h.cfg.RequestRetry != 1 || h.cfg.MaxRetryInterval != 30 || h.cfg.Routing.Strategy != "round-robin" || h.cfg.QuotaExceeded.SwitchProject {
		t.Fatalf("expected config to remain unchanged, got %#v", h.cfg)
	}
}

func TestPutRuntimeSettings_InvalidStrategyRejectedWithoutMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutPersistence(&config.Config{
		RequestRetry:     1,
		MaxRetryInterval: 30,
		Routing:          config.RoutingConfig{Strategy: "round-robin"},
		QuotaExceeded:    config.QuotaExceeded{},
	}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/runtime-settings", bytes.NewBufferString(`{"ws-auth":true,"request-retry":4,"max-retry-interval":90,"routing-strategy":"invalid","switch-project":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutRuntimeSettings(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if h.cfg.WebsocketAuth || h.cfg.RequestRetry != 1 || h.cfg.MaxRetryInterval != 30 || h.cfg.Routing.Strategy != "round-robin" || h.cfg.QuotaExceeded.SwitchProject {
		t.Fatalf("expected config to remain unchanged, got %#v", h.cfg)
	}
}

func TestGetAPIKeys_ReturnsItemsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutPersistence(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"first", "second"}}}, nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/api-keys", nil)

	h.GetAPIKeys(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		Items []string `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Items) != 2 || response.Items[0] != "first" || response.Items[1] != "second" {
		t.Fatalf("unexpected api key response: %#v", response)
	}
}

func TestPutAPIKeys_RequiresItemsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutPersistence(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"existing"}}}, nil)
	h.SetConfigSaver(func(*config.Config) error { return nil })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/api-keys", bytes.NewBufferString(`{"items":["next","final"]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutAPIKeys(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if len(h.cfg.APIKeys) != 2 || h.cfg.APIKeys[0] != "next" || h.cfg.APIKeys[1] != "final" {
		t.Fatalf("expected api keys to be replaced, got %#v", h.cfg.APIKeys)
	}
}
