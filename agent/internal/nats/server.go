// agent/internal/nats/server.go
//
// Embedded NATS server. The agent owns the broker so the rover keeps working
// when WAN is down; the proxy connection is a leaf-node link added later.
package nats

import (
	"context"
	"fmt"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/waypointos/waypoint/agent/internal/natsleaf"
)

type Options struct {
	// Port: -1 = random (test/dev). 4222 in production.
	Port int

	// Host: bind address. "127.0.0.1" by default.
	Host string

	// Logs enables nats-server's stdout/stderr logging. Tests leave this
	// false to keep output quiet; production should set it true so leaf
	// dial failures, retries, and auth rejections show up in journalctl.
	Logs bool

	// Users + NoAuthUser configure the embedded server's static auth list.
	// When Users is nil, the server runs with no auth (matches pre-modules
	// behavior). When set, NoAuthUser specifies which user unauthenticated
	// clients map to — keep this set to a Users[i].Username so the C++ core
	// and WS gateway, which connect without credentials, continue to work.
	Users      []*natsserver.User
	NoAuthUser string
}

type Server struct {
	ns   *natsserver.Server
	opts *natsserver.Options // startup options; cloned for each live SetUsers reload
}

// StartEmbedded boots an in-process nats-server on the configured port.
func StartEmbedded(opts Options) (*Server, error) {
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	natsOpts := &natsserver.Options{
		Host:           host,
		Port:           opts.Port,
		NoLog:          !opts.Logs,
		NoSigs:         true,
		MaxControlLine: 4096,
		// Basemap tiles travel over request/reply; the 1 MiB default is tight for
		// dense low-zoom vector tiles, so allow up to 4 MiB (matches the proxy).
		MaxPayload: 4 * 1024 * 1024,
	}
	if len(opts.Users) > 0 {
		natsOpts.Users = opts.Users
		natsOpts.NoAuthUser = opts.NoAuthUser
	}
	ns, err := natsserver.NewServer(natsOpts)
	if err != nil {
		return nil, fmt.Errorf("new nats server: %w", err)
	}
	// nats-server's NewServer leaves the logger nil; without an explicit
	// ConfigureLogger() call, every log message is silently dropped even
	// when NoLog=false. The library only auto-configures the logger on
	// config reload (server/reload.go), not at startup, so embedded
	// callers must opt in.
	if opts.Logs {
		ns.ConfigureLogger()
	}
	go ns.Start()
	return &Server{ns: ns, opts: natsOpts}, nil
}

// StartEmbeddedWithLeaf boots an in-process nats-server and attaches a leaf-node
// remote pointing at the Waypoint proxy hub using the provided credentials.
func StartEmbeddedWithLeaf(opts Options, proxyURL, accountJWT, userSeed string) (*Server, error) {
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	natsOpts := &natsserver.Options{
		Host:           host,
		Port:           opts.Port,
		NoLog:          !opts.Logs,
		NoSigs:         true,
		MaxControlLine: 4096,
		// Basemap tiles travel over request/reply; the 1 MiB default is tight for
		// dense low-zoom vector tiles, so allow up to 4 MiB (matches the proxy).
		MaxPayload: 4 * 1024 * 1024,
	}
	if len(opts.Users) > 0 {
		natsOpts.Users = opts.Users
		natsOpts.NoAuthUser = opts.NoAuthUser
	}
	if err := natsleaf.AttachToOptions(natsOpts, proxyURL, accountJWT, userSeed); err != nil {
		return nil, fmt.Errorf("attach leaf node: %w", err)
	}
	ns, err := natsserver.NewServer(natsOpts)
	if err != nil {
		return nil, fmt.Errorf("new nats server: %w", err)
	}
	// See StartEmbedded for why this is necessary even with NoLog=false.
	if opts.Logs {
		ns.ConfigureLogger()
	}
	go ns.Start()
	return &Server{ns: ns, opts: natsOpts}, nil
}

// WaitReady blocks until the server reports ready, or ctx fires.
func (s *Server) WaitReady(ctx context.Context) error {
	deadline, _ := ctx.Deadline()
	timeout := time.Until(deadline)
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if !s.ns.ReadyForConnections(timeout) {
		return fmt.Errorf("nats server not ready after %s", timeout)
	}
	return nil
}

// ClientURL returns a nats:// URL clients can dial.
func (s *Server) ClientURL() string {
	return s.ns.ClientURL()
}

// Server returns the underlying *natsserver.Server. Used by the alerts
// leaf-watcher to poll NumLeafNodes() for proxy-disconnect detection. Keep
// this surface minimal — callers should depend on the interface (alerts.LeafState)
// rather than the concrete type so the agent stays testable.
func (s *Server) Server() *natsserver.Server {
	return s.ns
}

// SetUsers hot-swaps the embedded server's auth user list without a restart, so
// a runtime-installed module authenticates immediately. Only Users changes; keep
// NoAuthUser set so credential-less clients (C++ core, WS gateway) stay connected.
func (s *Server) SetUsers(users []*natsserver.User, noAuthUser string) error {
	next := s.opts.Clone() // ReloadOptions takes ownership of what it's given
	next.Users = users
	next.NoAuthUser = noAuthUser
	if err := s.ns.ReloadOptions(next); err != nil {
		return fmt.Errorf("reload users: %w", err)
	}
	s.opts = next // baseline for the next reload
	return nil
}

// Shutdown stops the server. Safe to call multiple times.
func (s *Server) Shutdown() {
	s.ns.Shutdown()
	s.ns.WaitForShutdown()
}
