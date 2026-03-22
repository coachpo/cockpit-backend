package executor

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
