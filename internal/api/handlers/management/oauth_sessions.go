package management

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	oauthSessionTTL     = 10 * time.Minute
	maxOAuthStateLength = 128
)

var (
	errInvalidOAuthState      = errors.New("invalid oauth state")
	errUnsupportedOAuthFlow   = errors.New("unsupported oauth provider")
	errOAuthSessionNotPending = errors.New("oauth session is not pending")
)

type oauthSession struct {
	Provider  string
	Status    string
	Callback  *oauthCallbackPayload
	CreatedAt time.Time
	ExpiresAt time.Time
}

type oauthCallbackPayload struct {
	Code  string
	State string
	Error string
}

type oauthSessionStore struct {
	mu       sync.RWMutex
	ttl      time.Duration
	sessions map[string]oauthSession
}

func newOAuthSessionStore(ttl time.Duration) *oauthSessionStore {
	if ttl <= 0 {
		ttl = oauthSessionTTL
	}
	return &oauthSessionStore{
		ttl:      ttl,
		sessions: make(map[string]oauthSession),
	}
}

func (s *oauthSessionStore) purgeExpiredLocked(now time.Time) {
	for state, session := range s.sessions {
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			delete(s.sessions, state)
		}
	}
}

func (s *oauthSessionStore) Register(state, provider string) {
	state = strings.TrimSpace(state)
	provider = strings.ToLower(strings.TrimSpace(provider))
	if state == "" || provider == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	s.sessions[state] = oauthSession{
		Provider:  provider,
		Status:    "",
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
}

func (s *oauthSessionStore) SetError(state, message string) {
	state = strings.TrimSpace(state)
	message = strings.TrimSpace(message)
	if state == "" {
		return
	}
	if message == "" {
		message = "Authentication failed"
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok {
		return
	}
	session.Status = message
	session.ExpiresAt = now.Add(s.ttl)
	s.sessions[state] = session
}

func (s *oauthSessionStore) Complete(state string) {
	state = strings.TrimSpace(state)
	if state == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	delete(s.sessions, state)
}

func (s *oauthSessionStore) CompleteProvider(provider string) int {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return 0
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	removed := 0
	for state, session := range s.sessions {
		if strings.EqualFold(session.Provider, provider) {
			delete(s.sessions, state)
			removed++
		}
	}
	return removed
}

func (s *oauthSessionStore) Get(state string) (oauthSession, bool) {
	state = strings.TrimSpace(state)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	return session, ok
}

func (s *oauthSessionStore) StoreCallback(state, provider, code, errorMessage string) error {
	state = strings.TrimSpace(state)
	provider = strings.ToLower(strings.TrimSpace(provider))
	if err := ValidateOAuthState(state); err != nil {
		return err
	}
	if provider == "" {
		return errUnsupportedOAuthFlow
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok || session.Status != "" || !strings.EqualFold(session.Provider, provider) {
		return errOAuthSessionNotPending
	}
	session.Callback = &oauthCallbackPayload{
		Code:  strings.TrimSpace(code),
		State: state,
		Error: strings.TrimSpace(errorMessage),
	}
	session.ExpiresAt = now.Add(s.ttl)
	s.sessions[state] = session
	return nil
}

func (s *oauthSessionStore) ConsumeCallback(state, provider string) (oauthCallbackPayload, bool, error) {
	state = strings.TrimSpace(state)
	provider = strings.ToLower(strings.TrimSpace(provider))
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok {
		return oauthCallbackPayload{}, false, nil
	}
	if provider != "" && !strings.EqualFold(session.Provider, provider) {
		return oauthCallbackPayload{}, false, errOAuthSessionNotPending
	}
	if session.Callback == nil {
		return oauthCallbackPayload{}, false, nil
	}
	payload := *session.Callback
	session.Callback = nil
	session.ExpiresAt = now.Add(s.ttl)
	s.sessions[state] = session
	return payload, true, nil
}

func (s *oauthSessionStore) IsPending(state, provider string) bool {
	state = strings.TrimSpace(state)
	provider = strings.ToLower(strings.TrimSpace(provider))
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok {
		return false
	}
	if session.Status != "" {
		return false
	}
	if provider == "" {
		return true
	}
	return strings.EqualFold(session.Provider, provider)
}

var oauthSessions = newOAuthSessionStore(oauthSessionTTL)

func RegisterOAuthSession(state, provider string) { oauthSessions.Register(state, provider) }

func SetOAuthSessionError(state, message string) { oauthSessions.SetError(state, message) }

func CompleteOAuthSession(state string) { oauthSessions.Complete(state) }

func CompleteOAuthSessionsByProvider(provider string) int {
	return oauthSessions.CompleteProvider(provider)
}

func GetOAuthSession(state string) (provider string, status string, ok bool) {
	session, ok := oauthSessions.Get(state)
	if !ok {
		return "", "", false
	}
	return session.Provider, session.Status, true
}

func IsOAuthSessionPending(state, provider string) bool {
	return oauthSessions.IsPending(state, provider)
}

func StoreOAuthCallbackForPendingSession(provider, state, code, errorMessage string) error {
	canonicalProvider, err := NormalizeOAuthProvider(provider)
	if err != nil {
		return err
	}
	if !IsOAuthSessionPending(state, canonicalProvider) {
		return errOAuthSessionNotPending
	}
	return oauthSessions.StoreCallback(state, canonicalProvider, code, errorMessage)
}

func consumeOAuthCallback(state, provider string) (oauthCallbackPayload, bool, error) {
	return oauthSessions.ConsumeCallback(state, provider)
}

func ValidateOAuthState(state string) error {
	trimmed := strings.TrimSpace(state)
	if trimmed == "" {
		return fmt.Errorf("%w: empty", errInvalidOAuthState)
	}
	if len(trimmed) > maxOAuthStateLength {
		return fmt.Errorf("%w: too long", errInvalidOAuthState)
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return fmt.Errorf("%w: contains path separator", errInvalidOAuthState)
	}
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("%w: contains '..'", errInvalidOAuthState)
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("%w: invalid character", errInvalidOAuthState)
		}
	}
	return nil
}

func NormalizeOAuthProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex", "openai":
		return "codex", nil
	default:
		return "", errUnsupportedOAuthFlow
	}
}
