package handlers

import (
	"net/http"
	"testing"
)

func TestFilterUpstreamHeaders_RemovesConnectionScopedHeaders(t *testing.T) {
	src := http.Header{}
	src.Add("Connection", "keep-alive, x-hop-a, x-hop-b")
	src.Add("Connection", "x-hop-c")
	src.Set("Keep-Alive", "timeout=5")
	src.Set("X-Hop-A", "a")
	src.Set("X-Hop-B", "b")
	src.Set("X-Hop-C", "c")
	src.Set("X-Request-Id", "req-1")
	src.Set("Set-Cookie", "session=secret")

	filtered := FilterUpstreamHeaders(src)
	if filtered == nil {
		t.Fatalf("expected filtered headers, got nil")
	}

	requestID := filtered.Get("X-Request-Id")
	if requestID != "req-1" {
		t.Fatalf("expected X-Request-Id to be preserved, got %q", requestID)
	}

	blockedHeaderKeys := []string{
		"Connection",
		"Keep-Alive",
		"X-Hop-A",
		"X-Hop-B",
		"X-Hop-C",
		"Set-Cookie",
	}
	for _, key := range blockedHeaderKeys {
		value := filtered.Get(key)
		if value != "" {
			t.Fatalf("expected %s to be removed, got %q", key, value)
		}
	}
}

func TestFilterUpstreamHeaders_ReturnsNilWhenAllHeadersBlocked(t *testing.T) {
	src := http.Header{}
	src.Add("Connection", "x-hop-a")
	src.Set("X-Hop-A", "a")
	src.Set("Set-Cookie", "session=secret")

	filtered := FilterUpstreamHeaders(src)
	if filtered != nil {
		t.Fatalf("expected nil when all headers are filtered, got %#v", filtered)
	}
}

func TestDownstreamHeaders_AlwaysKeepsRateLimitHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("Retry-After", "30")
	src.Set("X-RateLimit-Remaining", "17")
	src.Set("X-Request-Id", "req-1")

	downstream := DownstreamHeaders(src, false)
	if downstream == nil {
		t.Fatalf("expected downstream headers, got nil")
	}
	if got := downstream.Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want %q", got, "30")
	}
	if got := downstream.Get("X-RateLimit-Remaining"); got != "17" {
		t.Fatalf("X-RateLimit-Remaining = %q, want %q", got, "17")
	}
	if got := downstream.Get("X-Request-Id"); got != "" {
		t.Fatalf("X-Request-Id should be excluded when passthrough is disabled, got %q", got)
	}
}

func TestDownstreamHeaders_WithPassthroughKeepsSafeHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("Retry-After", "30")
	src.Set("X-RateLimit-Remaining", "17")
	src.Set("X-Request-Id", "req-1")
	src.Set("Set-Cookie", "secret")

	downstream := DownstreamHeaders(src, true)
	if downstream == nil {
		t.Fatalf("expected downstream headers, got nil")
	}
	if got := downstream.Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want %q", got, "30")
	}
	if got := downstream.Get("X-RateLimit-Remaining"); got != "17" {
		t.Fatalf("X-RateLimit-Remaining = %q, want %q", got, "17")
	}
	if got := downstream.Get("X-Request-Id"); got != "req-1" {
		t.Fatalf("X-Request-Id = %q, want %q", got, "req-1")
	}
	if got := downstream.Get("Set-Cookie"); got != "" {
		t.Fatalf("Set-Cookie should still be filtered, got %q", got)
	}
}
