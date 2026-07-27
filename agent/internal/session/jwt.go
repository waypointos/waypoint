// Package session mints short-lived NATS user JWTs for the agent's local
// (LAN) dashboard sessions.
//
// On a local-only rover the embedded NATS server doesn't run in operator
// mode, so these JWTs aren't enforced by the local broker.
// They exist so that the same dashboard NATS bus client can speak either to
// the local agent or to the proxy hub without branching on auth shape, and so
// that a future operator-mode flip on the local NATS can drop in without
// re-plumbing the API surface.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// LocalSessionTTL is the lifetime of locally-minted dashboard session JWTs.
// Tighter than the proxy's 1h TTL because LAN auth is bootstrap-token-only
// and the dashboard refreshes well before expiry.
const LocalSessionTTL = 15 * time.Minute

// LocalIssuer signs dashboard session user JWTs with the rover's local
// account key. The account key is persisted to disk so restarts don't
// invalidate outstanding sessions.
type LocalIssuer struct {
	akp nkeys.KeyPair // account keypair (signs user claims)
}

// LoadOrCreate reads the cached account NKey seed from seedPath. If absent
// or unreadable, it generates a new account key and best-effort persists
// the seed to seedPath (mode 0600, parent dir 0750).
//
// A persist failure (e.g. unwritable parent) does NOT abort: the in-memory
// issuer is returned anyway. The cost is that the next restart will mint a
// new account pubkey and outstanding session JWTs will fail validation.
// Restart-stability is preferred-but-not-required in v1.
func LoadOrCreate(seedPath string) (*LocalIssuer, error) {
	if data, err := os.ReadFile(seedPath); err == nil {
		kp, err := nkeys.FromSeed(data)
		if err != nil {
			return nil, fmt.Errorf("seed at %s: %w", seedPath, err)
		}
		return &LocalIssuer{akp: kp}, nil
	}
	kp, err := nkeys.CreateAccount()
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	seed, err := kp.Seed()
	if err != nil {
		return nil, fmt.Errorf("extract seed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(seedPath), 0o750); err == nil {
		// Best-effort persist; see doc comment.
		_ = os.WriteFile(seedPath, seed, 0o600)
	}
	return &LocalIssuer{akp: kp}, nil
}

// PublicKey returns the account pubkey (A-prefixed). This is what a
// future operator-mode local NATS would list in its TrustedAccounts.
func (l *LocalIssuer) PublicKey() string {
	pk, _ := l.akp.PublicKey()
	return pk
}

// MintLocalSession mints a user JWT with admin scope on the given rover.
// Returns the JWT and the ephemeral user NKey seed. The dashboard presents
// (jwt, seed) when opening a NATS WebSocket — the seed signs the connect
// nonce, the JWT identifies the user.
func (l *LocalIssuer) MintLocalSession(roverID string, ttl time.Duration) (string, []byte, error) {
	ukp, err := nkeys.CreateUser()
	if err != nil {
		return "", nil, fmt.Errorf("create user: %w", err)
	}
	upk, err := ukp.PublicKey()
	if err != nil {
		return "", nil, fmt.Errorf("user pubkey: %w", err)
	}
	seed, err := ukp.Seed()
	if err != nil {
		return "", nil, fmt.Errorf("user seed: %w", err)
	}

	uc := jwt.NewUserClaims(upk)
	uc.Name = "local-session"
	uc.IssuedAt = time.Now().Unix()
	uc.Expires = uc.IssuedAt + int64(ttl.Seconds())
	uc.IssuerAccount = l.PublicKey()

	base := "waypoint." + roverID
	// Local sessions get admin scope on the single rover this agent owns.
	// Mirrors the proxy's "admin" role: cmd + rpc pub, full sub, plus
	// _INBOX.> for nats.go Request/Reply round-trips.
	uc.Permissions.Pub.Allow.Add(base+".cmd.>", base+".rpc.>", base+".module.>", "_INBOX.>")
	uc.Permissions.Sub.Allow.Add(base+".>", "_INBOX.>")

	tok, err := uc.Encode(l.akp)
	if err != nil {
		return "", nil, fmt.Errorf("encode user jwt: %w", err)
	}
	return tok, seed, nil
}
