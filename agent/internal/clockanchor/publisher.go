// Package clockanchor publishes the per-boot ClockAnchor on infra.clock so a
// recorder can map monotonic capture stamps to wall time.
package clockanchor

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/protocol/platform/stamp"
)

const (
	pollInterval    = 5 * time.Second
	reannounceEvery = time.Minute
)

type Publisher struct {
	nc      *nats.Conn
	roverID string
	synced  func() bool
}

func New(nc *nats.Conn, roverID string) *Publisher {
	return &Publisher{nc: nc, roverID: roverID, synced: NTPSynced}
}

// build derives the anchor from one dual-stamp capture: the wall-clock
// estimate of monotonic zero is wall minus monotonic.
func build(s *waypointv1.Stamp, synced bool) *waypointv1.ClockAnchor {
	wallNs := s.GetT().AsTime().UnixNano() - int64(s.GetMonoNs())
	return &waypointv1.ClockAnchor{
		BootId:         s.GetBootId(),
		WallAtMonoZero: timestamppb.New(time.Unix(0, wallNs).UTC()),
		NtpSynced:      synced,
		Stamp:          s,
	}
}

// shouldPublish: always on a sync-state change, otherwise on the re-announce
// cadence (NATS core has no retention; late subscribers need a recent copy).
func shouldPublish(cur, last bool, lastPub, now time.Time) bool {
	return cur != last || now.Sub(lastPub) >= reannounceEvery
}

func (p *Publisher) publishOnce(synced bool) {
	body, err := proto.Marshal(build(stamp.Now(), synced))
	if err != nil {
		return
	}
	_ = p.nc.Publish("waypoint."+p.roverID+".infra.clock", body)
}

// Run publishes once immediately, then on state change or re-announce, until
// ctx is cancelled. Publish errors are dropped; the next tick retries.
func (p *Publisher) Run(ctx context.Context) {
	last := p.synced()
	p.publishOnce(last)
	lastPub := time.Now()

	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cur := p.synced()
			if shouldPublish(cur, last, lastPub, now) {
				p.publishOnce(cur)
				last = cur
				lastPub = now
			}
		}
	}
}
