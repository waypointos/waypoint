// Package roverconn hands out one cached NATS connection per rover, each
// authenticated in that rover's OWN account. Proxy→rover RPC and publishes
// issued on these connections are intra-account, so the rover's leaf node
// carries them both ways — the path a cross-account Service import cannot take
// to a leaf-backed responder.
package roverconn

import (
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"
)

// CredFunc returns a fresh (userJWT, userSeed) for a connection in roverID's
// account. Typically wraps operator.MintRoverInternalUserJWT after resolving
// the rover's account pubkey.
type CredFunc func(roverID string) (userJWT, userSeed string, err error)

// Pool lazily creates and caches one connection per rover.
type Pool struct {
	natsURL string
	creds   CredFunc

	mu    sync.Mutex
	conns map[string]*nats.Conn
}

func NewPool(natsURL string, creds CredFunc) *Pool {
	return &Pool{natsURL: natsURL, creds: creds, conns: make(map[string]*nats.Conn)}
}

// Conn returns a connection in roverID's account, creating it on first use.
func (p *Pool) Conn(roverID string) (*nats.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.conns[roverID]; ok && !c.IsClosed() {
		return c, nil
	}
	jwt, seed, err := p.creds(roverID)
	if err != nil {
		return nil, fmt.Errorf("mint rover creds for %q: %w", roverID, err)
	}
	c, err := nats.Connect(p.natsURL, nats.UserJWTAndSeed(jwt, seed))
	if err != nil {
		return nil, fmt.Errorf("connect rover account %q: %w", roverID, err)
	}
	p.conns[roverID] = c
	return c, nil
}

// Remove closes and evicts roverID's cached connection. Call when a rover is
// deleted/revoked so a dead connection is not retried against a revoked account.
func (p *Pool) Remove(roverID string) {
	p.mu.Lock()
	c, ok := p.conns[roverID]
	delete(p.conns, roverID)
	p.mu.Unlock()
	if ok && c != nil {
		c.Close()
	}
}

// Close tears down every cached connection (process shutdown).
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.conns {
		c.Close()
		delete(p.conns, id)
	}
}
