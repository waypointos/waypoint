package sysinfo

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/protocol/platform/stamp"
)

// defaultInterval is the 1 Hz publish cadence specified in protocol/subjects.toml
// for telemetry.system. Tests reach into Publisher.interval to shorten this.
const defaultInterval = time.Second

// Publisher ticks at p.interval and publishes a SystemTelemetry message
// on waypoint.<roverID>.telemetry.system. Probes that report
// unavailable leave their fields absent in the published message — N/A
// is honest, not zero.
type Publisher struct {
	nc       *nats.Conn
	roverID  string
	probes   Probes
	image    func() ImageInfo
	rtt      func() (float64, bool)
	interval time.Duration
}

// New constructs a Publisher with the default 1 s interval.
//
//   - probes must be non-nil — use DefaultProbes() if you have no other
//     probe to wire.
//   - image may be nil; if so, ImageVersion and ImagePartition stay absent.
//   - rtt may be nil; if so, LinkRttMs stays absent. Pass
//     LeafRTTReader.LatestMs to surface the embedded nats-server's leaf
//     keepalive RTT measurement.
func New(nc *nats.Conn, roverID string, probes Probes, image func() ImageInfo, rtt func() (float64, bool)) *Publisher {
	return &Publisher{
		nc:       nc,
		roverID:  roverID,
		probes:   probes,
		image:    image,
		rtt:      rtt,
		interval: defaultInterval,
	}
}

// Run publishes one SystemTelemetry per interval until ctx is cancelled.
// Publish errors are not logged (would spam at 1 Hz on a flapping
// transport) and do not stop the loop — the next tick tries again.
func (p *Publisher) Run(ctx context.Context) {
	subject := "waypoint." + p.roverID + ".telemetry.system"

	t := time.NewTicker(p.interval)
	defer t.Stop()

	publishOnce := func(now time.Time) {
		msg := &waypointv1.SystemTelemetry{T: timestamppb.New(now), Stamp: stamp.Now()}
		if cpu, ok := p.probes.CPUPercent(); ok {
			msg.CpuPercent = &cpu
		}
		if temp, ok := p.probes.TempC(); ok {
			msg.TemperatureC = &temp
		}
		if used, total, ok := p.probes.MemoryBytes(); ok {
			msg.MemoryUsedBytes = &used
			msg.MemoryTotalBytes = &total
		}
		if up, ok := p.probes.UptimeSeconds(); ok {
			msg.UptimeSeconds = &up
		}
		if p.image != nil {
			info := p.image()
			if info.Version != "" {
				msg.ImageVersion = &info.Version
			}
			if info.Partition != "" {
				msg.ImagePartition = &info.Partition
			}
		}
		if p.rtt != nil {
			if rttMs, ok := p.rtt(); ok {
				msg.LinkRttMs = &rttMs
			}
		}
		body, err := proto.Marshal(msg)
		if err != nil {
			return // marshal error on a fixed schema is effectively impossible
		}
		_ = p.nc.Publish(subject, body)
	}

	// Publish once immediately so the dashboard goes Online without
	// waiting a full interval after agent start.
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
