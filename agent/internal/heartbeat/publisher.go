// Package heartbeat publishes the agent -> core heartbeat protobuf on
// waypoint.<rover-id>.infra.heartbeat at 200 ms cadence. Core's
// watchdog (500 ms timeout) kicks on every received message; the
// proxy's fleet-liveness consumer also listens here for last_seen
// updates.
//
// Cadence is intentionally ~2.5x faster than the watchdog timeout so
// scheduler jitter (Go ticker drift, busy laptop, etc.) doesn't push a
// single late tick across the timeout boundary and falsely trip the
// watchdog. Rule of thumb: heartbeat period <= timeout / 2.
package heartbeat

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

const defaultInterval = 200 * time.Millisecond

// Publisher emits a Heartbeat protobuf on waypoint.<roverID>.infra.heartbeat
// every interval. Publish errors are silently dropped (would spam every
// tick on a flapping transport) and do not stop the loop.
type Publisher struct {
	nc       *nats.Conn
	roverID  string
	interval time.Duration
}

// New constructs a Publisher with the default 200 ms interval.
func New(nc *nats.Conn, roverID string) *Publisher {
	return &Publisher{
		nc:       nc,
		roverID:  roverID,
		interval: defaultInterval,
	}
}

// Run publishes a Heartbeat every interval until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	subject := "waypoint." + p.roverID + ".infra.heartbeat"
	t := time.NewTicker(p.interval)
	defer t.Stop()

	publishOnce := func(now time.Time) {
		msg := &waypointv1.Heartbeat{T: timestamppb.New(now)}
		body, err := proto.Marshal(msg)
		if err != nil {
			return
		}
		_ = p.nc.Publish(subject, body)
	}

	// Publish once immediately so core leaves Safe mode quickly after start.
	publishOnce(time.Now())

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			publishOnce(now)
		}
	}
}
