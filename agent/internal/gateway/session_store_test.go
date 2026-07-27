package gateway

import (
	"testing"
	"time"
)

func TestSessionStore_IssueValidUnknownExpired(t *testing.T) {
	s := newSessionStore(50 * time.Millisecond)

	tok, err := s.issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if tok == "" {
		t.Fatal("issued an empty token")
	}
	if !s.valid(tok) {
		t.Fatal("freshly issued token should be valid")
	}
	if s.valid("") || s.valid("not-a-real-token") {
		t.Fatal("unknown/empty tokens must be invalid")
	}

	// Two issues must yield distinct tokens.
	tok2, _ := s.issue()
	if tok2 == tok {
		t.Fatal("issue must return distinct tokens")
	}

	// After the TTL, the token expires.
	time.Sleep(60 * time.Millisecond)
	if s.valid(tok) {
		t.Fatal("token should be expired after its TTL")
	}
}
