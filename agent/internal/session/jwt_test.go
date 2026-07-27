package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
)

func TestLocalIssuer_LoadOrCreate_PersistsSeed(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "local-account.nk")

	first, err := LoadOrCreate(seedPath)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	pk1 := first.PublicKey()
	if pk1 == "" {
		t.Fatal("empty pubkey after first load")
	}

	second, err := LoadOrCreate(seedPath)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	pk2 := second.PublicKey()
	if pk1 != pk2 {
		t.Fatalf("pubkey mismatch across loads: %q vs %q", pk1, pk2)
	}
}

func TestLocalIssuer_LoadOrCreate_CreatesIfMissing(t *testing.T) {
	dir := t.TempDir()
	// Use a nested subdir to ensure MkdirAll runs.
	seedPath := filepath.Join(dir, "nested", "local-account.nk")

	iss, err := LoadOrCreate(seedPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	pk := iss.PublicKey()
	if !strings.HasPrefix(pk, "A") {
		t.Fatalf("expected account pubkey (A-prefixed), got %q", pk)
	}

	info, err := os.Stat(seedPath)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}
	// Mode 0600 — secrets on disk should never be world-readable.
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("seed file mode %#o, want 0600", mode)
	}
}

func TestLocalSession_GrantsModulePublish(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "local-account.nk")
	iss, err := LoadOrCreate(seedPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	tok, _, err := iss.MintLocalSession("rover-01", time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	uc, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	want := "waypoint.rover-01.module.>"
	found := false
	for _, s := range uc.Permissions.Pub.Allow {
		if s == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pub.allow missing %q (got %v)", want, uc.Permissions.Pub.Allow)
	}
}

func TestLocalIssuer_MintLocalSession_AdminScope(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "local-account.nk")
	iss, err := LoadOrCreate(seedPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	roverID := "rover-test"
	ttl := 30 * time.Minute

	tok, seed, err := iss.MintLocalSession(roverID, ttl)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if tok == "" {
		t.Fatal("empty jwt")
	}
	if len(seed) == 0 {
		t.Fatal("empty seed")
	}

	claims, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got := claims.IssuerAccount; got != iss.PublicKey() {
		t.Fatalf("issuer account = %q, want %q", got, iss.PublicKey())
	}

	wantPub := map[string]bool{
		"waypoint." + roverID + ".cmd.>": false,
		"waypoint." + roverID + ".rpc.>": false,
		"_INBOX.>":                       false,
	}
	for _, s := range claims.Permissions.Pub.Allow {
		if _, ok := wantPub[s]; ok {
			wantPub[s] = true
		}
	}
	for s, found := range wantPub {
		if !found {
			t.Errorf("pub.allow missing %q (got %v)", s, claims.Permissions.Pub.Allow)
		}
	}

	wantSub := map[string]bool{
		"waypoint." + roverID + ".>": false,
		"_INBOX.>":                   false,
	}
	for _, s := range claims.Permissions.Sub.Allow {
		if _, ok := wantSub[s]; ok {
			wantSub[s] = true
		}
	}
	for s, found := range wantSub {
		if !found {
			t.Errorf("sub.allow missing %q (got %v)", s, claims.Permissions.Sub.Allow)
		}
	}

	// TTL should match within a couple seconds (issuance jitter).
	gotTTL := claims.Expires - claims.IssuedAt
	wantTTL := int64(ttl.Seconds())
	if gotTTL != wantTTL {
		t.Errorf("ttl seconds = %d, want %d", gotTTL, wantTTL)
	}
}
