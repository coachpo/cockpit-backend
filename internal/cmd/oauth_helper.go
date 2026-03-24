package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coachpo/cockpit-backend/internal/browser"
)

const (
	defaultOAuthHelperProvider      = "codex"
	defaultOAuthHelperCallbackAddr  = "localhost:1455"
	defaultOAuthHelperCallbackPath  = "/auth/callback"
	defaultOAuthHelperSessionTTL    = 10 * time.Minute
	oauthSessionCreateEndpoint      = "/api/oauth-sessions"
	oauthCallbackForwardErrorPrefix = "oauth helper"
)

type oauthHelperNextAction string

const (
	oauthHelperActionContinue     oauthHelperNextAction = "continue"
	oauthHelperActionChangeTarget oauthHelperNextAction = "change-target"
	oauthHelperActionQuit         oauthHelperNextAction = "quit"
)

type OAuthHelperOptions struct {
	Target         string
	Provider       string
	NoBrowser      bool
	CallbackAddr   string
	CallbackPath   string
	SessionTimeout time.Duration
	HTTPClient     *http.Client
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	BrowserOpen    func(string) error
	BrowserReady   func() bool
	Listen         func(network, address string) (net.Listener, error)
}

type oauthHelperSessionCreateRequest struct {
	Provider string `json:"provider"`
}

type oauthHelperSessionCreateResponse struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Error  string `json:"error"`
}

type oauthCallbackForwarder struct {
	server        *http.Server
	closeOnce     sync.Once
	stateMu       sync.RWMutex
	expectedState string
	resultCh      chan string
	errCh         chan error
}

func RunOAuthHelper(ctx context.Context, opts OAuthHelperOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeOAuthHelperOptions(opts)
	promptReader := bufio.NewReader(opts.Stdin)

	provider := strings.TrimSpace(opts.Provider)
	if provider == "" {
		provider = defaultOAuthHelperProvider
	}
	currentTarget := strings.TrimSpace(opts.Target)

	for {
		target, err := resolveOAuthHelperTarget(currentTarget, promptReader, opts.Stdout)
		if err != nil {
			return err
		}
		if err := runOAuthHelperRound(ctx, opts, target, provider); err != nil {
			return err
		}

		nextAction, err := promptOAuthHelperNextAction(promptReader, opts.Stdout)
		if err != nil {
			return err
		}

		switch nextAction {
		case oauthHelperActionContinue:
			currentTarget = target
		case oauthHelperActionChangeTarget:
			currentTarget = ""
		case oauthHelperActionQuit:
			return nil
		default:
			return fmt.Errorf("%s: unsupported next action %q", oauthCallbackForwardErrorPrefix, nextAction)
		}
	}
}

func runOAuthHelperRound(ctx context.Context, opts OAuthHelperOptions, target, provider string) error {
	targetCallbackURL := composeOAuthHelperTargetURL(target, defaultOAuthHelperCallbackPath)
	forwarder, callbackURL, err := startOAuthCallbackForwarder(opts.CallbackAddr, opts.CallbackPath, targetCallbackURL, "", opts.Listen)
	if err != nil {
		return err
	}
	defer forwarder.Close(context.Background())

	session, err := startOAuthHelperSession(ctx, opts.HTTPClient, target, provider)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("%s: empty oauth session response", oauthCallbackForwardErrorPrefix)
	}
	forwarder.SetExpectedState(session.State)

	_, _ = fmt.Fprintf(opts.Stdout, "Using Cockpit target: %s\n", target)
	_, _ = fmt.Fprintf(opts.Stdout, "Listening for OAuth callback on %s\n", callbackURL)
	_, _ = fmt.Fprintf(opts.Stdout, "Forwarding accepted callbacks to %s\n", targetCallbackURL)
	_, _ = fmt.Fprintf(opts.Stdout, "Open this URL to continue authentication:\n%s\n", session.URL)

	if !opts.NoBrowser && opts.BrowserReady() {
		if errOpen := opts.BrowserOpen(session.URL); errOpen != nil {
			_, _ = fmt.Fprintf(opts.Stderr, "Unable to open browser automatically: %v\n", errOpen)
		} else {
			_, _ = fmt.Fprintln(opts.Stdout, "Opened the OAuth URL in your browser.")
		}
	}

	_, _ = fmt.Fprintln(opts.Stdout, "Waiting for OAuth callback...")
	forwardedURL, err := forwarder.Wait(ctx, opts.SessionTimeout)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(opts.Stdout, "OAuth callback received successfully.")
	_, _ = fmt.Fprintf(opts.Stdout, "Forwarded OAuth callback to %s\n", forwardedURL)
	return nil
}

func normalizeOAuthHelperOptions(opts OAuthHelperOptions) OAuthHelperOptions {
	if opts.Provider == "" {
		opts.Provider = defaultOAuthHelperProvider
	}
	if strings.TrimSpace(opts.CallbackAddr) == "" {
		opts.CallbackAddr = defaultOAuthHelperCallbackAddr
	}
	if strings.TrimSpace(opts.CallbackPath) == "" {
		opts.CallbackPath = defaultOAuthHelperCallbackPath
	}
	if opts.SessionTimeout <= 0 {
		opts.SessionTimeout = defaultOAuthHelperSessionTTL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.BrowserOpen == nil {
		opts.BrowserOpen = browser.OpenURL
	}
	if opts.BrowserReady == nil {
		opts.BrowserReady = browser.IsAvailable
	}
	if opts.Listen == nil {
		opts.Listen = net.Listen
	}
	return opts
}

func resolveOAuthHelperTarget(raw string, reader *bufio.Reader, out io.Writer) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		input, err := readOAuthHelperPromptLine(reader, out, "Cockpit backend URL: ")
		if err != nil {
			return "", fmt.Errorf("%s: read target: %w", oauthCallbackForwardErrorPrefix, err)
		}
		trimmed = input
	}
	if trimmed == "" {
		return "", fmt.Errorf("%s: backend target is required", oauthCallbackForwardErrorPrefix)
	}
	return normalizeOAuthHelperTarget(trimmed)
}

func promptOAuthHelperNextAction(reader *bufio.Reader, out io.Writer) (oauthHelperNextAction, error) {
	for {
		input, err := readOAuthHelperPromptLine(reader, out, "Next action? [Enter=continue, t=change target, q=quit]: ")
		if err != nil {
			return "", fmt.Errorf("%s: read next action: %w", oauthCallbackForwardErrorPrefix, err)
		}

		switch normalized := strings.ToLower(strings.TrimSpace(input)); normalized {
		case "", "c", "continue":
			return oauthHelperActionContinue, nil
		case "t", "target", "change", "change-target", "change target":
			return oauthHelperActionChangeTarget, nil
		case "q", "quit", "exit":
			return oauthHelperActionQuit, nil
		default:
			_, _ = fmt.Fprintln(out, "Please choose Enter, t, or q.")
		}
	}
}

func readOAuthHelperPromptLine(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	if out != nil && prompt != "" {
		_, _ = fmt.Fprint(out, prompt)
	}
	if reader == nil {
		return "", io.EOF
	}
	input, err := reader.ReadString('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return "", err
		}
		if strings.TrimSpace(input) == "" {
			return "", io.EOF
		}
	}
	return strings.TrimSpace(input), nil
}

func normalizeOAuthHelperTarget(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%s: backend target is required", oauthCallbackForwardErrorPrefix)
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%s: parse backend target: %w", oauthCallbackForwardErrorPrefix, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s: backend target must use http or https", oauthCallbackForwardErrorPrefix)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("%s: backend target host is required", oauthCallbackForwardErrorPrefix)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func composeOAuthHelperTargetURL(target, suffix string) string {
	base := strings.TrimRight(strings.TrimSpace(target), "/")
	if base == "" {
		return suffix
	}
	if strings.HasPrefix(suffix, "/") {
		return base + suffix
	}
	return base + "/" + suffix
}

func startOAuthHelperSession(ctx context.Context, client *http.Client, target, provider string) (*oauthHelperSessionCreateResponse, error) {
	requestBody, err := json.Marshal(oauthHelperSessionCreateRequest{
		Provider: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: encode oauth session request: %w", oauthCallbackForwardErrorPrefix, err)
	}
	endpoint := composeOAuthHelperTargetURL(target, oauthSessionCreateEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("%s: build oauth session request: %w", oauthCallbackForwardErrorPrefix, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: create oauth session: %w", oauthCallbackForwardErrorPrefix, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read oauth session response: %w", oauthCallbackForwardErrorPrefix, err)
	}
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("%s: oauth session request failed: %s", oauthCallbackForwardErrorPrefix, message)
	}

	var session oauthHelperSessionCreateResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("%s: decode oauth session response: %w", oauthCallbackForwardErrorPrefix, err)
	}
	if strings.TrimSpace(session.State) == "" || strings.TrimSpace(session.URL) == "" {
		return nil, fmt.Errorf("%s: oauth session response missing state or url", oauthCallbackForwardErrorPrefix)
	}
	return &session, nil
}

func startOAuthCallbackForwarder(addr, callbackPath, targetCallbackURL, expectedState string, listen func(network, address string) (net.Listener, error)) (*oauthCallbackForwarder, string, error) {
	listener, err := listen("tcp", addr)
	if err != nil {
		return nil, "", fmt.Errorf("%s: listen on %s: %w", oauthCallbackForwardErrorPrefix, addr, err)
	}
	forwarder := &oauthCallbackForwarder{
		expectedState: expectedState,
		resultCh:      make(chan string, 1),
		errCh:         make(chan error, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		state := strings.TrimSpace(r.URL.Query().Get("state"))
		if currentState := forwarder.ExpectedState(); currentState != "" && state != currentState {
			http.Error(w, "state does not match active oauth session", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(r.URL.Query().Get("code")) == "" && strings.TrimSpace(r.URL.Query().Get("error")) == "" && strings.TrimSpace(r.URL.Query().Get("error_description")) == "" {
			http.Error(w, "oauth callback missing code or error", http.StatusBadRequest)
			return
		}
		forwardURL := targetCallbackURL
		if rawQuery := strings.TrimSpace(r.URL.RawQuery); rawQuery != "" {
			forwardURL += "?" + rawQuery
		}
		w.Header().Set("Cache-Control", "no-store")
		select {
		case forwarder.resultCh <- forwardURL:
		default:
		}
		http.Redirect(w, r, forwardURL, http.StatusFound)
		go forwarder.Close(context.Background())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	forwarder.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	go func() {
		if errServe := forwarder.server.Serve(listener); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
			select {
			case forwarder.errCh <- errServe:
			default:
			}
		}
	}()

	return forwarder, "http://" + listener.Addr().String() + callbackPath, nil
}

func (f *oauthCallbackForwarder) Wait(ctx context.Context, timeout time.Duration) (string, error) {
	if f == nil {
		return "", fmt.Errorf("%s: callback forwarder is unavailable", oauthCallbackForwardErrorPrefix)
	}
	if timeout <= 0 {
		timeout = defaultOAuthHelperSessionTTL
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case forwardedURL := <-f.resultCh:
		return forwardedURL, nil
	case err := <-f.errCh:
		return "", fmt.Errorf("%s: callback listener failed: %w", oauthCallbackForwardErrorPrefix, err)
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", fmt.Errorf("%s: timeout waiting for oauth callback", oauthCallbackForwardErrorPrefix)
	}
}

func (f *oauthCallbackForwarder) SetExpectedState(state string) {
	if f == nil {
		return
	}
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	f.expectedState = strings.TrimSpace(state)
}

func (f *oauthCallbackForwarder) ExpectedState() string {
	if f == nil {
		return ""
	}
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	return f.expectedState
}

func (f *oauthCallbackForwarder) Close(ctx context.Context) {
	if f == nil || f.server == nil {
		return
	}
	f.closeOnce.Do(func() {
		shutdownCtx := ctx
		if shutdownCtx == nil {
			shutdownCtx = context.Background()
		}
		shutdownCtx, cancel := context.WithTimeout(shutdownCtx, 2*time.Second)
		defer cancel()
		_ = f.server.Shutdown(shutdownCtx)
	})
}
