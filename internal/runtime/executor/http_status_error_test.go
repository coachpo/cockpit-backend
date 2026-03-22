package executor

import (
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPStatusErr_PreservesHeadersAndRetryAfter(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Retry-After", "12")
	headers.Set("X-RateLimit-Remaining", "0")

	err := newHTTPStatusErr(http.StatusTooManyRequests, []byte(`{"error":"rate limit"}`), headers)

	if got := err.StatusCode(); got != http.StatusTooManyRequests {
		t.Fatalf("StatusCode() = %d, want %d", got, http.StatusTooManyRequests)
	}
	if err.RetryAfter() == nil || *err.RetryAfter() != 12*time.Second {
		t.Fatalf("RetryAfter() = %#v, want 12s", err.RetryAfter())
	}
	out := err.Headers()
	if got := out.Get("Retry-After"); got != "12" {
		t.Fatalf("Headers().Get(Retry-After) = %q, want %q", got, "12")
	}
	if got := out.Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("Headers().Get(X-RateLimit-Remaining) = %q, want %q", got, "0")
	}
	headers.Set("X-RateLimit-Remaining", "99")
	if got := out.Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("cloned headers mutated to %q, want %q", got, "0")
	}
}
