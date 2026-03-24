package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeOAuthHelperTarget_AddsSchemeAndTrimsTrailingSlash(t *testing.T) {
	got, err := normalizeOAuthHelperTarget("cockpit.example.com:8443/")
	if err != nil {
		t.Fatalf("normalizeOAuthHelperTarget() error = %v", err)
	}
	if got != "http://cockpit.example.com:8443" {
		t.Fatalf("expected normalized target, got %q", got)
	}
}

func TestResolveOAuthHelperTarget_ReadsSinglePromptLine(t *testing.T) {
	stdout := &bytes.Buffer{}
	got, err := resolveOAuthHelperTarget("", bufio.NewReader(strings.NewReader("http://127.0.0.1:38317\nignored\n")), stdout)
	if err != nil {
		t.Fatalf("resolveOAuthHelperTarget() error = %v", err)
	}
	if got != "http://127.0.0.1:38317" {
		t.Fatalf("expected prompt target to use only first line, got %q", got)
	}
	if !strings.Contains(stdout.String(), "Cockpit backend URL:") {
		t.Fatalf("expected prompt output, got %q", stdout.String())
	}
}

func TestPromptOAuthHelperNextAction_DefaultsToContinue(t *testing.T) {
	action, err := promptOAuthHelperNextAction(bufio.NewReader(strings.NewReader("\n")), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("promptOAuthHelperNextAction() error = %v", err)
	}
	if action != oauthHelperActionContinue {
		t.Fatalf("expected default continue action, got %q", action)
	}
}

func TestPromptOAuthHelperNextAction_AllowsChangeTargetAndQuit(t *testing.T) {
	changeAction, err := promptOAuthHelperNextAction(bufio.NewReader(strings.NewReader("t\n")), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("promptOAuthHelperNextAction() change-target error = %v", err)
	}
	if changeAction != oauthHelperActionChangeTarget {
		t.Fatalf("expected change-target action, got %q", changeAction)
	}

	quitAction, err := promptOAuthHelperNextAction(bufio.NewReader(strings.NewReader("q\n")), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("promptOAuthHelperNextAction() quit error = %v", err)
	}
	if quitAction != oauthHelperActionQuit {
		t.Fatalf("expected quit action, got %q", quitAction)
	}
}

func TestPromptOAuthHelperNextAction_RePromptsAfterInvalidInput(t *testing.T) {
	stdout := &bytes.Buffer{}
	action, err := promptOAuthHelperNextAction(bufio.NewReader(strings.NewReader("wat\nq\n")), stdout)
	if err != nil {
		t.Fatalf("promptOAuthHelperNextAction() error = %v", err)
	}
	if action != oauthHelperActionQuit {
		t.Fatalf("expected quit action after invalid input, got %q", action)
	}
	if !strings.Contains(stdout.String(), "Please choose Enter, t, or q.") {
		t.Fatalf("expected invalid choice guidance, got %q", stdout.String())
	}
}

func TestRunOAuthHelper_StartsSessionAndRedirectsCallback(t *testing.T) {
	t.Parallel()

	type createRequest struct {
		Provider            string `json:"provider"`
		LocalCallbackHelper bool   `json:"local_callback_helper"`
	}

	var (
		mu                sync.Mutex
		capturedCreateReq createRequest
		callbackQuery     url.Values
		createCalls       int
	)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oauthSessionCreateEndpoint:
			createCalls++
			defer func() { _ = r.Body.Close() }()
			if err := json.NewDecoder(r.Body).Decode(&capturedCreateReq); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"state":  "state-123",
				"url":    "https://auth.example.test/authorize?state=state-123",
			})
		case defaultOAuthHelperCallbackPath:
			mu.Lock()
			callbackQuery = r.URL.Query()
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("backend callback ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	var listenAddr string
	listen := func(network, address string) (net.Listener, error) {
		listener, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			listenAddr = listener.Addr().String()
		}
		return listener, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	done := make(chan error, 1)

	go func() {
		done <- RunOAuthHelper(ctx, OAuthHelperOptions{
			Target:         backend.URL,
			NoBrowser:      false,
			CallbackAddr:   "127.0.0.1:0",
			SessionTimeout: 5 * time.Second,
			Stdin:          strings.NewReader("q\n"),
			Stdout:         stdout,
			Stderr:         stderr,
			BrowserReady:   func() bool { return true },
			BrowserOpen:    func(string) error { return nil },
			Listen:         listen,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for listenAddr == "" {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for oauth helper listener")
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := http.Get("http://" + listenAddr + defaultOAuthHelperCallbackPath + "?state=state-123&code=auth-code")
	if err != nil {
		t.Fatalf("invoke local callback: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected redirected backend callback status 200, got %d", resp.StatusCode)
	}

	if err := <-done; err != nil {
		t.Fatalf("RunOAuthHelper() error = %v; stderr=%s", err, stderr.String())
	}
	if capturedCreateReq.Provider != defaultOAuthHelperProvider {
		t.Fatalf("expected provider %q, got %+v", defaultOAuthHelperProvider, capturedCreateReq)
	}
	if !capturedCreateReq.LocalCallbackHelper {
		t.Fatalf("expected helper mode request body, got %+v", capturedCreateReq)
	}
	if createCalls != 1 {
		t.Fatalf("expected one oauth session create call, got %d", createCalls)
	}
	mu.Lock()
	defer mu.Unlock()
	if callbackQuery.Get("state") != "state-123" || callbackQuery.Get("code") != "auth-code" {
		t.Fatalf("expected backend callback query to be forwarded, got %v", callbackQuery)
	}
	if !strings.Contains(stdout.String(), "Listening for OAuth callback") || !strings.Contains(stdout.String(), "OAuth callback received successfully.") || !strings.Contains(stdout.String(), "Forwarded OAuth callback") || !strings.Contains(stdout.String(), "Next action? [Enter=continue, t=change target, q=quit]:") {
		t.Fatalf("expected helper progress output, got %q", stdout.String())
	}
	if count := strings.Count(stdout.String(), "https://auth.example.test/authorize?state=state-123"); count != 1 {
		t.Fatalf("expected oauth url to be printed once, got %d occurrence(s) in %q", count, stdout.String())
	}
}

func TestRunOAuthHelper_DefaultContinueStartsAnotherRoundWithSameTarget(t *testing.T) {
	t.Parallel()

	type createRequest struct {
		Provider            string `json:"provider"`
		LocalCallbackHelper bool   `json:"local_callback_helper"`
	}

	var (
		mu          sync.Mutex
		createCalls int
		states      []string
		queries     []url.Values
	)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oauthSessionCreateEndpoint:
			mu.Lock()
			createCalls++
			callNumber := createCalls
			mu.Unlock()
			defer func() { _ = r.Body.Close() }()
			var req createRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if !req.LocalCallbackHelper {
				t.Fatalf("expected helper flag in create request, got %+v", req)
			}
			state := fmt.Sprintf("state-%d", callNumber)
			mu.Lock()
			states = append(states, state)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"state":  state,
				"url":    "https://auth.example.test/authorize?state=" + state,
			})
		case defaultOAuthHelperCallbackPath:
			mu.Lock()
			queries = append(queries, r.URL.Query())
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("backend callback ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	var (
		listenMu    sync.Mutex
		listenAddrs []string
	)
	listen := func(network, address string) (net.Listener, error) {
		listener, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			listenMu.Lock()
			listenAddrs = append(listenAddrs, listener.Addr().String())
			listenMu.Unlock()
		}
		return listener, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	done := make(chan error, 1)

	go func() {
		done <- RunOAuthHelper(ctx, OAuthHelperOptions{
			Target:         backend.URL,
			NoBrowser:      true,
			CallbackAddr:   "127.0.0.1:0",
			SessionTimeout: 5 * time.Second,
			Stdin:          strings.NewReader("\nq\n"),
			Stdout:         stdout,
			Stderr:         stderr,
			BrowserReady:   func() bool { return false },
			Listen:         listen,
		})
	}()

	waitFor := func(predicate func() bool, message string) {
		deadline := time.Now().Add(3 * time.Second)
		for !predicate() {
			if time.Now().After(deadline) {
				t.Fatal(message)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	waitFor(func() bool {
		listenMu.Lock()
		defer listenMu.Unlock()
		return len(listenAddrs) >= 1
	}, "timed out waiting for first helper listener")
	listenMu.Lock()
	firstAddr := listenAddrs[0]
	listenMu.Unlock()
	resp1, err := http.Get("http://" + firstAddr + defaultOAuthHelperCallbackPath + "?state=state-1&code=code-1")
	if err != nil {
		t.Fatalf("invoke first callback: %v", err)
	}
	_ = resp1.Body.Close()

	waitFor(func() bool {
		listenMu.Lock()
		defer listenMu.Unlock()
		return len(listenAddrs) >= 2
	}, "timed out waiting for second helper listener")
	listenMu.Lock()
	secondAddr := listenAddrs[1]
	listenMu.Unlock()
	resp2, err := http.Get("http://" + secondAddr + defaultOAuthHelperCallbackPath + "?state=state-2&code=code-2")
	if err != nil {
		t.Fatalf("invoke second callback: %v", err)
	}
	_ = resp2.Body.Close()

	if err := <-done; err != nil {
		t.Fatalf("RunOAuthHelper() error = %v; stderr=%s", err, stderr.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if createCalls != 2 {
		t.Fatalf("expected two oauth session create calls, got %d", createCalls)
	}
	if len(queries) != 2 || queries[0].Get("state") != "state-1" || queries[1].Get("state") != "state-2" {
		t.Fatalf("expected forwarded callbacks for both rounds, got %#v", queries)
	}
	if count := strings.Count(stdout.String(), "Next action? [Enter=continue, t=change target, q=quit]:"); count != 2 {
		t.Fatalf("expected next action prompt twice, got %d in %q", count, stdout.String())
	}
}
