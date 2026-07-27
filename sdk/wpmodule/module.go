// Package wpmodule is the Waypoint module SDK: it absorbs the runtime
// boilerplate every module needs (creds, connect, health, stats, shutdown)
// and provides typed clients for the agent's broker capabilities and
// servers for the standard component APIs.
package wpmodule

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	natsgo "github.com/nats-io/nats.go"
)

type Options struct {
	ID string // module id (required); WAYPOINT_MODULE_ID overrides

	// StatsInterval is the ModuleStats heartbeat period. Zero means 5 s.
	StatsInterval time.Duration
}

// M is the running module handle passed to the setup callback.
type M struct {
	id      string
	roverID string
	nc      *natsgo.Conn
	start   time.Time
	done    chan struct{}

	mu    sync.Mutex
	stops []func()
}

func (m *M) ID() string       { return m.id }
func (m *M) RoverID() string  { return m.roverID }
func (m *M) NC() *natsgo.Conn { return m.nc } // escape hatch

// Done is closed when Run begins shutting down. Module goroutines (tickers,
// control loops) should select on it so they stop before the NATS drain.
func (m *M) Done() <-chan struct{} { return m.done }

// addStop registers a cleanup Run invokes (LIFO) at shutdown, before drain.
func (m *M) addStop(fn func()) {
	m.mu.Lock()
	m.stops = append(m.stops, fn)
	m.mu.Unlock()
}

func (m *M) runStops() {
	m.mu.Lock()
	stops := m.stops
	m.stops = nil
	m.mu.Unlock()
	for i := len(stops) - 1; i >= 0; i-- {
		stops[i]()
	}
}

// Subject returns the fully qualified subject for a leaf in this module's
// sandbox, e.g. Subject("arm.state") -> waypoint.<rover>.module.<id>.arm.state.
func (m *M) Subject(leaf string) string {
	return fmt.Sprintf("waypoint.%s.module.%s.%s", m.roverID, m.id, leaf)
}

func (m *M) sandboxPrefix() string {
	return fmt.Sprintf("waypoint.%s.module.%s.", m.roverID, m.id)
}

func (m *M) checkScope(subject string) error {
	if !strings.HasPrefix(subject, m.sandboxPrefix()) {
		return fmt.Errorf("wpmodule: subject %q outside module sandbox %s>", subject, m.sandboxPrefix())
	}
	return nil
}

// Publish/Subscribe/Request are sandbox-scoped; use NC() to step outside
// deliberately (the server still enforces the minted ACL).
func (m *M) Publish(subject string, data []byte) error {
	if err := m.checkScope(subject); err != nil {
		return err
	}
	return m.nc.Publish(subject, data)
}

func (m *M) Subscribe(subject string, cb natsgo.MsgHandler) (*natsgo.Subscription, error) {
	if err := m.checkScope(subject); err != nil {
		return nil, err
	}
	return m.nc.Subscribe(subject, cb)
}

func (m *M) Request(subject string, data []byte, timeout time.Duration) (*natsgo.Msg, error) {
	if err := m.checkScope(subject); err != nil {
		return nil, err
	}
	return m.nc.Request(subject, data, timeout)
}

// Run connects, serves health and stats, invokes setup, then blocks until
// ctx is cancelled or SIGINT/SIGTERM arrives. It drains the connection on
// the way out.
func Run(ctx context.Context, opts Options, setup func(m *M) error) error {
	id := envOr("WAYPOINT_MODULE_ID", opts.ID)
	if id == "" {
		return fmt.Errorf("wpmodule: Options.ID (or WAYPOINT_MODULE_ID) is required")
	}
	roverID := os.Getenv("WAYPOINT_ROVER_ID")
	if roverID == "" {
		return fmt.Errorf("wpmodule: WAYPOINT_ROVER_ID is required")
	}
	url := envOr("WAYPOINT_NATS_URL", natsgo.DefaultURL)

	natsOpts := []natsgo.Option{natsgo.MaxReconnects(-1), natsgo.ReconnectWait(2 * time.Second)}
	if credsPath := os.Getenv("WAYPOINT_MODULE_CREDS"); credsPath != "" {
		user, pass, err := loadCredsEnv(credsPath)
		if err != nil {
			return fmt.Errorf("wpmodule: creds: %w", err)
		}
		natsOpts = append(natsOpts, natsgo.UserInfo(user, pass))
	}
	nc, err := natsgo.Connect(url, natsOpts...)
	if err != nil {
		return fmt.Errorf("wpmodule: nats connect %s: %w", url, err)
	}
	defer nc.Drain() //nolint:errcheck

	m := &M{id: id, roverID: roverID, nc: nc, start: time.Now(), done: make(chan struct{})}

	healthSub, err := nc.Subscribe(m.Subject("health.ready"), func(msg *natsgo.Msg) {
		_ = msg.Respond([]byte("ok"))
	})
	if err != nil {
		return fmt.Errorf("wpmodule: health responder: %w", err)
	}
	defer healthSub.Unsubscribe() //nolint:errcheck

	interval := opts.StatsInterval
	if interval == 0 {
		interval = 5 * time.Second
	}
	stopStats := m.startStats(interval)
	defer stopStats()

	if setup != nil {
		if err := setup(m); err != nil {
			return fmt.Errorf("wpmodule: setup: %w", err)
		}
	}
	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()
	// Shutdown order: signal module goroutines, stop component servers
	// (LIFO), then the deferred stats/health teardown and the drain.
	close(m.done)
	m.runStops()
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
