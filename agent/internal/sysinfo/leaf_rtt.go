package sysinfo

import (
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// leafzReader is the slice of nats-server we depend on for leaf-node
// RTT visibility. The embedded *natsserver.Server.Leafz() satisfies it;
// tests substitute a fake to avoid spinning up an actual leaf for an
// RTT-parsing unit test.
type leafzReader interface {
	Leafz(opts *natsserver.LeafzOptions) (*natsserver.Leafz, error)
}

// LeafRTTReader exposes the leaf-node round-trip time measured by the
// embedded nats-server's PING/PONG keepalive. No request/reply
// scaffolding is needed at the application level — nats-server already
// tracks this internally for every active leaf.
type LeafRTTReader struct {
	server leafzReader
}

// NewLeafRTTReader wraps the embedded nats-server's *Server.
func NewLeafRTTReader(s leafzReader) *LeafRTTReader {
	return &LeafRTTReader{server: s}
}

// LatestMs returns the most recently measured leaf-node RTT in
// milliseconds. Reports (_, false) when no leaf is connected or the
// keepalive hasn't measured RTT yet — the Publisher renders that as an
// absent field in SystemTelemetry, which the dashboard surfaces as N/A.
//
// nats-server stores LeafInfo.RTT as a Go time.Duration formatted via
// its String() method (e.g. "1.234ms", "523µs"). ParseDuration round-trips
// that format exactly.
func (r *LeafRTTReader) LatestMs() (float64, bool) {
	leafz, err := r.server.Leafz(nil)
	if err != nil || leafz == nil || len(leafz.Leafs) == 0 {
		return 0, false
	}
	rttStr := leafz.Leafs[0].RTT
	if rttStr == "" {
		return 0, false
	}
	d, err := time.ParseDuration(rttStr)
	if err != nil {
		return 0, false
	}
	return float64(d) / float64(time.Millisecond), true
}
