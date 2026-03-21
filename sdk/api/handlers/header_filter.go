package handlers

import (
	"net/http"
	"strings"
)

// hopByHopHeaders lists RFC 7230 Section 6.1 hop-by-hop headers that MUST NOT
// be forwarded by proxies, plus security-sensitive headers that should not leak.
var hopByHopHeaders = map[string]struct{}{
	// RFC 7230 hop-by-hop
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	// Security-sensitive
	"Set-Cookie": {},
	// CPA-managed (set by handlers, not upstream)
	"Content-Length":   {},
	"Content-Encoding": {},
}

// FilterUpstreamHeaders returns a copy of src with hop-by-hop and security-sensitive
// headers removed. Returns nil if src is nil or empty after filtering.
func FilterUpstreamHeaders(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	connectionScoped := connectionScopedHeaders(src)
	dst := make(http.Header)
	for key, values := range src {
		canonicalKey := http.CanonicalHeaderKey(key)
		if _, blocked := hopByHopHeaders[canonicalKey]; blocked {
			continue
		}
		if _, scoped := connectionScoped[canonicalKey]; scoped {
			continue
		}
		dst[key] = values
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

// FilterRateLimitHeaders keeps only downstream-useful rate limit metadata.
// It is intentionally narrower than FilterUpstreamHeaders and is used even when
// full header passthrough is disabled.
func FilterRateLimitHeaders(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	dst := make(http.Header)
	for key, values := range src {
		canonicalKey := http.CanonicalHeaderKey(key)
		lowerKey := strings.ToLower(canonicalKey)
		if canonicalKey != "Retry-After" &&
			!strings.Contains(lowerKey, "ratelimit") &&
			!strings.Contains(lowerKey, "rate-limit") {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

// DownstreamHeaders returns the headers safe to expose to downstream clients.
// Rate limit headers are always forwarded; broader safe upstream headers require
// passthrough-headers to be enabled.
func DownstreamHeaders(src http.Header, passthroughAll bool) http.Header {
	filtered := FilterUpstreamHeaders(src)
	if passthroughAll {
		return filtered
	}
	return FilterRateLimitHeaders(filtered)
}

func connectionScopedHeaders(src http.Header) map[string]struct{} {
	scoped := make(map[string]struct{})
	for _, rawValue := range src.Values("Connection") {
		for _, token := range strings.Split(rawValue, ",") {
			headerName := strings.TrimSpace(token)
			if headerName == "" {
				continue
			}
			scoped[http.CanonicalHeaderKey(headerName)] = struct{}{}
		}
	}
	return scoped
}

// WriteUpstreamHeaders writes filtered upstream headers to the gin response writer.
// Headers already set by CPA (e.g., Content-Type) are NOT overwritten.
func WriteUpstreamHeaders(dst http.Header, src http.Header) {
	if src == nil {
		return
	}
	for key, values := range src {
		// Don't overwrite headers already set by CPA handlers
		if dst.Get(key) != "" {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}
