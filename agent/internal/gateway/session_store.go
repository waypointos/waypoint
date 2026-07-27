package gateway

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

// localSessionCookieTTL is how long an auto-issued local session stays valid.
// WithLocalSession re-issues on a fresh document load, so this only bounds an
// idle/closed-tab session.
const localSessionCookieTTL = 7 * 24 * time.Hour

// sessionStore holds hashes of issued local session tokens. The raw token
// lives only in the client's wp_local_token cookie; we keep just its SHA-256,
// so dumping the store never yields a usable credential. In-memory by design:
// on agent restart all sessions drop and the next dashboard load silently
// re-issues (see Gateway.WithLocalSession).
type sessionStore struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[[32]byte]time.Time // token hash → expiry
}

func newSessionStore(ttl time.Duration) *sessionStore {
	return &sessionStore{ttl: ttl, m: make(map[[32]byte]time.Time)}
}

// issue mints a new opaque token, records its hash with an expiry, and returns
// the raw token to set as the cookie value.
func (s *sessionStore) issue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.m[sha256.Sum256([]byte(tok))] = time.Now().Add(s.ttl)
	return tok, nil
}

// valid reports whether tok is a known, unexpired session token.
func (s *sessionStore) valid(tok string) bool {
	if tok == "" {
		return false
	}
	h := sha256.Sum256([]byte(tok))
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.m[h]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.m, h)
		return false
	}
	return true
}

// pruneLocked drops expired entries; the caller holds the lock.
func (s *sessionStore) pruneLocked() {
	now := time.Now()
	for h, exp := range s.m {
		if now.After(exp) {
			delete(s.m, h)
		}
	}
}
