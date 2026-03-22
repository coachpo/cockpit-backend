package management

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
	"github.com/coachpo/cockpit-backend/internal/registry"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

var lastRefreshKeys = []string{"last_refresh", "lastRefresh", "last_refreshed_at", "lastRefreshedAt"}

const managedStoreAttribute = "store_managed"

func extractLastRefreshTimestamp(meta map[string]any) (time.Time, bool) {
	if len(meta) == 0 {
		return time.Time{}, false
	}
	for _, key := range lastRefreshKeys {
		if val, ok := meta[key]; ok {
			if ts, ok1 := parseLastRefreshValue(val); ok1 {
				return ts, true
			}
		}
	}
	return time.Time{}, false
}

func parseLastRefreshValue(v any) (time.Time, bool) {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return time.Time{}, false
		}
		layouts := []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00"}
		for _, layout := range layouts {
			if ts, err := time.Parse(layout, s); err == nil {
				return ts.UTC(), true
			}
		}
		if unix, err := strconv.ParseInt(s, 10, 64); err == nil && unix > 0 {
			return time.Unix(unix, 0).UTC(), true
		}
	case float64:
		if val <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(val), 0).UTC(), true
	case int64:
		if val <= 0 {
			return time.Time{}, false
		}
		return time.Unix(val, 0).UTC(), true
	case int:
		if val <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(val), 0).UTC(), true
	case json.Number:
		if i, err := val.Int64(); err == nil && i > 0 {
			return time.Unix(i, 0).UTC(), true
		}
	}
	return time.Time{}, false
}

func authEmail(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if v, ok := auth.Metadata["email"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	if auth.Attributes != nil {
		if v := strings.TrimSpace(auth.Attributes["email"]); v != "" {
			return v
		}
		if v := strings.TrimSpace(auth.Attributes["account_email"]); v != "" {
			return v
		}
	}
	return ""
}

func authAttribute(auth *coreauth.Auth, key string) string {
	if auth == nil || len(auth.Attributes) == 0 {
		return ""
	}
	return auth.Attributes[key]
}

func isRuntimeOnlyAuth(auth *coreauth.Auth) bool {
	if auth == nil || len(auth.Attributes) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(auth.Attributes["runtime_only"]), "true")
}

func isManagedStoredAuth(auth *coreauth.Auth) bool {
	if auth == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(authAttribute(auth, managedStoreAttribute)), "true") {
		return true
	}
	return strings.TrimSpace(authAttribute(auth, "path")) != ""
}

func authIDForName(name string) string {
	id := strings.TrimSpace(filepath.Base(name))
	if runtime.GOOS == "windows" {
		id = strings.ToLower(id)
	}
	return id
}

func buildAuthAttributes(metadata map[string]any) map[string]string {
	attributes := map[string]string{}
	if metadata == nil {
		return attributes
	}
	if prefix, ok := metadata["prefix"].(string); ok && strings.TrimSpace(prefix) != "" {
		attributes["prefix"] = strings.TrimSpace(prefix)
	}
	if proxyURL, ok := metadata["proxy_url"].(string); ok && strings.TrimSpace(proxyURL) != "" {
		attributes["proxy_url"] = strings.TrimSpace(proxyURL)
	}
	if note, ok := metadata["note"].(string); ok && strings.TrimSpace(note) != "" {
		attributes["note"] = strings.TrimSpace(note)
	}
	attributes[managedStoreAttribute] = "true"
	switch value := metadata["priority"].(type) {
	case string:
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			attributes["priority"] = trimmed
		}
	case float64:
		attributes["priority"] = strconv.Itoa(int(value))
	case int:
		attributes["priority"] = strconv.Itoa(value)
	}
	if provider, _ := metadata["type"].(string); strings.EqualFold(strings.TrimSpace(provider), "codex") {
		if idTokenRaw, ok := metadata["id_token"].(string); ok && strings.TrimSpace(idTokenRaw) != "" {
			if claims, errParse := codex.ParseJWTToken(idTokenRaw); errParse == nil && claims != nil {
				if pt := strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType); pt != "" {
					attributes["plan_type"] = pt
				}
			}
		}
	}
	return attributes
}

func syncManagedAuthModels(auth *coreauth.Auth) {
	if auth == nil || strings.TrimSpace(auth.ID) == "" {
		return
	}
	if auth.Disabled {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
		return
	}
	provider := strings.ToLower(strings.TrimSpace(auth.Provider))
	var models []*registry.ModelInfo
	switch provider {
	case "codex":
		planType := ""
		if auth.Attributes != nil {
			planType = strings.ToLower(strings.TrimSpace(auth.Attributes["plan_type"]))
		}
		switch planType {
		case "free":
			models = registry.GetCodexFreeModels()
		case "team", "business", "go":
			models = registry.GetCodexTeamModels()
		case "plus":
			models = registry.GetCodexPlusModels()
		default:
			models = registry.GetCodexProModels()
		}
	default:
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
		return
	}
	if len(models) == 0 {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
		return
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, provider, models)
}
