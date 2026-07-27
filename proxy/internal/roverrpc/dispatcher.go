// Package roverrpc subscribes per-rover responders on each rover's own-account
// connection for rover→proxy RPC subjects (rpc.basemap_tile, rpc.ice_servers).
// Intra-account so request/reply works: the cross-account Stream import the
// sessions account uses to firehose telemetry can carry a request but cannot
// route the reply back to a leaf-backed publisher, the same dead end documented
// on roverconn.Pool.
package roverrpc

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/nats-io/nats.go"
)

// Handler answers one rover→proxy RPC subject. Implementations are pure:
// given the request bytes, produce reply bytes. Dispatcher owns the NATS
// subscribe/respond plumbing and the per-rover subject prefix.
type Handler interface {
	// Leaf is the per-rover subject suffix this handler answers, e.g.
	// "rpc.basemap_tile". Dispatcher subscribes to "waypoint.<roverID>." + Leaf().
	Leaf() string
	// Handle returns the protobuf-marshaled reply bytes for req. A nil return
	// is treated as "no reply" and the request times out on the caller; only
	// return nil when the handler genuinely has nothing to say.
	Handle(req []byte) []byte
}

// conns is the slice of roverconn.Pool roverrpc needs: a per-rover account
// connection it can subscribe on. Narrowed to an interface so tests can stub it.
type conns interface {
	Conn(roverID string) (*nats.Conn, error)
}

// IDLister enumerates known rovers for EnsureAll. main.go wraps db.RoversRepo
// in a few lines so this package can stay free of the DB dependency.
type IDLister interface {
	IDs(ctx context.Context) ([]string, error)
}

// Dispatcher tracks per-rover subscriptions for the registered handlers.
type Dispatcher struct {
	conns    conns
	handlers []Handler

	mu   sync.Mutex
	subs map[string][]*nats.Subscription
}

// New builds a dispatcher. The handlers list is fixed for the dispatcher's
// lifetime; per-rover subscriptions are added/removed via Ensure/Remove.
func New(c conns, handlers ...Handler) *Dispatcher {
	return &Dispatcher{conns: c, handlers: handlers, subs: make(map[string][]*nats.Subscription)}
}

// Ensure subscribes every handler on roverID's own-account connection.
// Idempotent: a second call for the same rover is a no-op. On partial failure
// it rolls back the subscriptions it created so a retry starts clean.
func (d *Dispatcher) Ensure(roverID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.subs[roverID]; ok {
		return nil
	}
	nc, err := d.conns.Conn(roverID)
	if err != nil {
		return fmt.Errorf("roverrpc: conn %q: %w", roverID, err)
	}
	subs := make([]*nats.Subscription, 0, len(d.handlers))
	for _, h := range d.handlers {
		h := h
		subj := fmt.Sprintf("waypoint.%s.%s", roverID, h.Leaf())
		s, err := nc.Subscribe(subj, func(m *nats.Msg) {
			if reply := h.Handle(m.Data); reply != nil {
				if err := m.Respond(reply); err != nil {
					log.Printf("roverrpc: respond %s: %v", subj, err)
				}
			}
		})
		if err != nil {
			for _, x := range subs {
				_ = x.Unsubscribe()
			}
			return fmt.Errorf("roverrpc: subscribe %s: %w", subj, err)
		}
		subs = append(subs, s)
	}
	d.subs[roverID] = subs
	return nil
}

// Remove unsubscribes every handler for roverID. Safe to call for a rover that
// was never ensured. The underlying connection is owned by roverconn.Pool, so
// Remove on the pool is the caller's responsibility (the delete handler does
// both, in order: dispatcher Remove, then pool Remove).
func (d *Dispatcher) Remove(roverID string) {
	d.mu.Lock()
	subs := d.subs[roverID]
	delete(d.subs, roverID)
	d.mu.Unlock()
	for _, s := range subs {
		_ = s.Unsubscribe()
	}
}

// EnsureAll iterates lister and calls Ensure for every rover. Per-rover errors
// are logged and skipped: one bad rover must not block startup, the same
// trade-off restoreRoverAccounts already takes for the JWT restore path.
func (d *Dispatcher) EnsureAll(ctx context.Context, lister IDLister) error {
	ids, err := lister.IDs(ctx)
	if err != nil {
		return fmt.Errorf("roverrpc: list rovers: %w", err)
	}
	for _, id := range ids {
		if err := d.Ensure(id); err != nil {
			log.Printf("roverrpc: ensure %q: %v", id, err)
		}
	}
	return nil
}
