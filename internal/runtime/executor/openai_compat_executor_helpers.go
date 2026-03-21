package executor

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	cliproxyauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/sjson"
)

// Refresh is a no-op for API-key based compatibility providers.
func (e *OpenAICompatExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	log.Debugf("openai compat executor: refresh called")
	_ = ctx
	return auth, nil
}

func (e *OpenAICompatExecutor) resolveCredentials(auth *cliproxyauth.Auth) (baseURL, apiKey string) {
	if auth == nil {
		return "", ""
	}
	if auth.Attributes != nil {
		baseURL = strings.TrimSpace(auth.Attributes["base_url"])
		apiKey = strings.TrimSpace(auth.Attributes["api_key"])
	}
	return
}

func (e *OpenAICompatExecutor) resolveCompatConfig(auth *cliproxyauth.Auth) *config.OpenAICompatibility {
	if auth == nil || e.cfg == nil {
		return nil
	}
	candidates := make([]string, 0, 3)
	if auth.Attributes != nil {
		if v := strings.TrimSpace(auth.Attributes["compat_name"]); v != "" {
			candidates = append(candidates, v)
		}
		if v := strings.TrimSpace(auth.Attributes["provider_key"]); v != "" {
			candidates = append(candidates, v)
		}
	}
	if v := strings.TrimSpace(auth.Provider); v != "" {
		candidates = append(candidates, v)
	}
	for i := range e.cfg.OpenAICompatibility {
		compat := &e.cfg.OpenAICompatibility[i]
		for _, candidate := range candidates {
			if candidate != "" && strings.EqualFold(strings.TrimSpace(candidate), compat.Name) {
				return compat
			}
		}
	}
	return nil
}

func (e *OpenAICompatExecutor) overrideModel(payload []byte, model string) []byte {
	if len(payload) == 0 || model == "" {
		return payload
	}
	payload, _ = sjson.SetBytes(payload, "model", model)
	return payload
}

type statusErr struct {
	code       int
	msg        string
	retryAfter *time.Duration
	headers    http.Header
}

func (e statusErr) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("status %d", e.code)
}
func (e statusErr) StatusCode() int            { return e.code }
func (e statusErr) RetryAfter() *time.Duration { return e.retryAfter }
func (e statusErr) Headers() http.Header {
	if e.headers == nil && e.retryAfter == nil {
		return nil
	}
	headers := make(http.Header)
	for key, values := range e.headers {
		headers[key] = append([]string(nil), values...)
	}
	if e.retryAfter != nil {
		seconds := int(math.Ceil(e.retryAfter.Seconds()))
		if seconds < 0 {
			seconds = 0
		}
		if headers.Get("Retry-After") == "" {
			headers.Set("Retry-After", strconv.Itoa(seconds))
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func newHTTPStatusErr(statusCode int, body []byte, headers http.Header) statusErr {
	err := statusErr{code: statusCode, msg: string(body)}
	if headers != nil {
		err.headers = headers.Clone()
		if retryAfter := parseHTTPRetryAfter(headers); retryAfter != nil {
			err.retryAfter = retryAfter
		}
	}
	return err
}

func parseHTTPRetryAfter(headers http.Header) *time.Duration {
	if headers == nil {
		return nil
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		dur := time.Duration(seconds) * time.Second
		return &dur
	}
	if when, err := http.ParseTime(raw); err == nil {
		dur := time.Until(when)
		if dur < 0 {
			dur = 0
		}
		return &dur
	}
	return nil
}
