// Package natsunix exposes the agent's embedded NATS server (which listens on
// TCP loopback in-process) on a Unix domain socket so the C++ core daemon
// can speak the text-line NATS protocol without TCP/TLS.
package natsunix

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
)

type Config struct {
	SocketPath string // e.g. "/run/waypoint/nats.sock" or "/tmp/waypoint-nats.sock"
	BackendURL string // e.g. "nats://127.0.0.1:4222" — the agent's embedded NATS TCP URL
}

type Relay struct {
	cfg      Config
	listener net.Listener
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func Start(ctx context.Context, cfg Config) (*Relay, error) {
	if cfg.SocketPath == "" || cfg.BackendURL == "" {
		return nil, fmt.Errorf("natsunix: SocketPath and BackendURL are required")
	}
	_ = os.Remove(cfg.SocketPath) // stale socket from previous run

	l, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", cfg.SocketPath, err)
	}
	if err := os.Chmod(cfg.SocketPath, 0660); err != nil {
		slog.Warn(fmt.Sprintf("natsunix: chmod %s: %v", cfg.SocketPath, err))
	}

	ctx, cancel := context.WithCancel(ctx)
	r := &Relay{cfg: cfg, listener: l, cancel: cancel}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		<-ctx.Done()
		_ = l.Close()
	}()

	r.wg.Add(1)
	go r.acceptLoop(ctx)

	slog.Info(fmt.Sprintf("natsunix: relay listening on %s → %s", cfg.SocketPath, cfg.BackendURL))
	return r, nil
}

func (r *Relay) acceptLoop(ctx context.Context) {
	defer r.wg.Done()
	for {
		c, err := r.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn(fmt.Sprintf("natsunix: accept: %v", err))
			return
		}
		go r.handle(ctx, c)
	}
}

func (r *Relay) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	// Strip the nats:// scheme.
	backendAddr := r.cfg.BackendURL
	if len(backendAddr) > 7 && backendAddr[:7] == "nats://" {
		backendAddr = backendAddr[7:]
	}
	upstream, err := net.Dial("tcp", backendAddr)
	if err != nil {
		slog.Warn(fmt.Sprintf("natsunix: dial backend %s: %v", backendAddr, err))
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, c); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, upstream); done <- struct{}{} }()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

func (r *Relay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	_ = os.Remove(r.cfg.SocketPath)
}
