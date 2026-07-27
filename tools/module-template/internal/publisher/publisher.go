// Package publisher owns the NATS connection: it answers health probes and
// publishes an ExampleStats snapshot on a timer.
package publisher

import (
	"context"
	"fmt"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	examplev1 "github.com/waypointos/waypoint-module-example/protocol/gen/go"
)

// Publisher publishes module telemetry and replies to health probes.
type Publisher struct {
	nc           *natsgo.Conn
	statsSubject string
	message      string

	mu       sync.Mutex
	interval time.Duration
	count    int64
}

// New wires the publisher to the bus and starts replying to health probes on
// waypoint.<rover>.module.example.health.ready.
func New(nc *natsgo.Conn, roverID, message string) (*Publisher, error) {
	p := &Publisher{
		nc:           nc,
		statsSubject: fmt.Sprintf("waypoint.%s.module.example.stats", roverID),
		message:      message,
		interval:     5 * time.Second,
	}
	health := fmt.Sprintf("waypoint.%s.module.example.health.ready", roverID)
	if _, err := nc.Subscribe(health, func(m *natsgo.Msg) { _ = m.Respond([]byte("ok")) }); err != nil {
		return nil, fmt.Errorf("subscribe health: %w", err)
	}
	return p, nil
}

// SetInterval overrides the publish cadence (default 5s).
func (p *Publisher) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	p.mu.Lock()
	p.interval = d
	p.mu.Unlock()
}

// Run publishes immediately, then every interval, until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	p.mu.Lock()
	interval := p.interval
	p.mu.Unlock()
	t := time.NewTicker(interval)
	defer t.Stop()
	p.publishOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.publishOnce()
		}
	}
}

func (p *Publisher) publishOnce() {
	p.mu.Lock()
	p.count++
	stats := &examplev1.ExampleStats{T: timestamppb.Now(), Count: p.count}
	if p.message != "" {
		msg := p.message
		stats.Message = &msg
	}
	p.mu.Unlock()
	body, err := proto.Marshal(stats)
	if err != nil {
		return
	}
	_ = p.nc.Publish(p.statsSubject, body)
}
