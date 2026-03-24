package management

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coachpo/cockpit-backend/internal/auth/codex"
)

const (
	oauthSessionTTL     = 10 * time.Minute
	maxOAuthStateLength = 128
	oauthStatusPending  = "pending"
	oauthStatusComplete = "complete"
	oauthStatusError    = "error"
)

var (
	errInvalidOAuthState    = errors.New("invalid oauth state")
	errUnsupportedOAuthFlow = errors.New("unsupported oauth provider")
	errOAuthSessionNotFound = errors.New("oauth session not found")
	errOAuthSessionExpired  = errors.New("oauth session expired")
)

type oauthSession struct {
	State       string
	Provider    string
	RedirectURI string
	PKCECodes   *codex.PKCECodes
	Status      string
	Error       string
	AuthFile    string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type oauthSessionStore struct {
	mu       sync.RWMutex
	ttl      time.Duration
	sessions map[string]oauthSession
	expired  map[string]time.Time
}

func newOAuthSessionStore(ttl time.Duration) *oauthSessionStore {
	if ttl <= 0 {
		ttl = oauthSessionTTL
	}
	return &oauthSessionStore{
		ttl:      ttl,
		sessions: make(map[string]oauthSession),
		expired:  make(map[string]time.Time),
	}
}

func (s *oauthSessionStore) purgeExpiredLocked(now time.Time) {
	for state, session := range s.sessions {
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			delete(s.sessions, state)
			s.expired[state] = now.Add(s.ttl)
		}
	}
	for state, until := range s.expired {
		if now.After(until) {
			delete(s.expired, state)
		}
	}
}

func normalizeOAuthSession(session oauthSession) oauthSession {
	session.State = strings.TrimSpace(session.State)
	session.Provider = strings.ToLower(strings.TrimSpace(session.Provider))
	session.RedirectURI = strings.TrimSpace(session.RedirectURI)
	session.Error = strings.TrimSpace(session.Error)
	session.AuthFile = strings.TrimSpace(session.AuthFile)
	if session.Status == "" {
		session.Status = oauthStatusPending
	}
	return session
}

func (s *oauthSessionStore) Register(session oauthSession) error {
	session = normalizeOAuthSession(session)
	if err := ValidateOAuthState(session.State); err != nil {
		return err
	}
	if session.Provider == "" {
		return errUnsupportedOAuthFlow
	}
	now := time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.ExpiresAt.IsZero() {
		session.ExpiresAt = now.Add(s.ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	delete(s.expired, session.State)
	s.sessions[session.State] = session
	return nil
}

func (s *oauthSessionStore) Get(state string) (oauthSession, error) {
	state = strings.TrimSpace(state)
	if err := ValidateOAuthState(state); err != nil {
		return oauthSession{}, err
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	if _, ok := s.expired[state]; ok {
		return oauthSession{}, errOAuthSessionExpired
	}
	session, ok := s.sessions[state]
	if !ok {
		return oauthSession{}, errOAuthSessionNotFound
	}
	return session, nil
}

func (s *oauthSessionStore) Remove(state string) {
	state = strings.TrimSpace(state)
	if state == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, state)
	delete(s.expired, state)
}

func (s *oauthSessionStore) SetError(state, message string) error {
	state = strings.TrimSpace(state)
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Authentication failed"
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	if _, ok := s.expired[state]; ok {
		return errOAuthSessionExpired
	}
	session, ok := s.sessions[state]
	if !ok {
		return errOAuthSessionNotFound
	}
	session.Status = oauthStatusError
	session.Error = message
	session.ExpiresAt = now.Add(s.ttl)
	s.sessions[state] = session
	return nil
}

func (s *oauthSessionStore) Complete(state, authFile string) error {
	state = strings.TrimSpace(state)
	authFile = strings.TrimSpace(authFile)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	if _, ok := s.expired[state]; ok {
		return errOAuthSessionExpired
	}
	session, ok := s.sessions[state]
	if !ok {
		return errOAuthSessionNotFound
	}
	session.Status = oauthStatusComplete
	session.Error = ""
	session.AuthFile = authFile
	session.ExpiresAt = now.Add(s.ttl)
	s.sessions[state] = session
	return nil
}

func (s *oauthSessionStore) IsPending(state, provider string) bool {
	session, err := s.Get(state)
	if err != nil {
		return false
	}
	if session.Status != oauthStatusPending {
		return false
	}
	if provider == "" {
		return true
	}
	return strings.EqualFold(session.Provider, strings.TrimSpace(provider))
}

var oauthSessions = newOAuthSessionStore(oauthSessionTTL)

func RegisterOAuthSession(state, provider string) error {
	return oauthSessions.Register(oauthSession{State: state, Provider: provider, Status: oauthStatusPending})
}

func RegisterOAuthSessionWithRedirect(state, provider, redirectURI string, pkceCodes *codex.PKCECodes) error {
	return oauthSessions.Register(oauthSession{
		State:       state,
		Provider:    provider,
		RedirectURI: redirectURI,
		PKCECodes:   pkceCodes,
		Status:      oauthStatusPending,
	})
}

func LoadOAuthSession(state string) (oauthSession, error) { return oauthSessions.Get(state) }

func SetOAuthSessionError(state, message string) error { return oauthSessions.SetError(state, message) }

func CompleteOAuthSession(state string) error { return oauthSessions.Complete(state, "") }

func CompleteOAuthSessionWithAuthFile(state, authFile string) error {
	return oauthSessions.Complete(state, authFile)
}

func RemoveOAuthSession(state string) { oauthSessions.Remove(state) }

func IsOAuthSessionPending(state, provider string) bool {
	return oauthSessions.IsPending(state, provider)
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
	case "codex":
		return "codex", nil
	default:
		return "", errUnsupportedOAuthFlow
	}
}
