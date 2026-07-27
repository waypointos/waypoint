package modules

import (
	"context"
	"fmt"
	"sync"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// uplinkMirror republishes a connectivity module's UplinkTelemetry from its
// sandboxed module.<id>.uplink subject onto the platform telemetry.uplink rail.
// The agent is the only writer of telemetry.*; modules reach it solely through
// a declared `provides = ["uplink"]` capability. Payloads that don't decode as
// UplinkTelemetry are dropped so the typed rail can't be polluted.
type uplinkMirror struct {
	nc      *natsgo.Conn
	roverID string

	mu   sync.Mutex
	subs map[string]*natsgo.Subscription
}

func newUplinkMirror(nc *natsgo.Conn, roverID string) *uplinkMirror {
	return &uplinkMirror{nc: nc, roverID: roverID, subs: map[string]*natsgo.Subscription{}}
}

func (u *uplinkMirror) start(_ context.Context, moduleID string) error {
	if u == nil || u.nc == nil {
		return fmt.Errorf("uplink mirror: nil nats conn")
	}
	src := fmt.Sprintf("waypoint.%s.module.%s.uplink", u.roverID, moduleID)
	dst := fmt.Sprintf("waypoint.%s.telemetry.uplink", u.roverID)
	sub, err := u.nc.Subscribe(src, func(m *natsgo.Msg) {
		if err := proto.Unmarshal(m.Data, &waypointv1.UplinkTelemetry{}); err != nil {
			return // drop anything that isn't well-formed protobuf
		}
		_ = u.nc.Publish(dst, m.Data)
	})
	if err != nil {
		return fmt.Errorf("uplink mirror: subscribe %s: %w", src, err)
	}
	u.mu.Lock()
	if old := u.subs[moduleID]; old != nil {
		_ = old.Unsubscribe()
	}
	u.subs[moduleID] = sub
	u.mu.Unlock()
	return nil
}

func (u *uplinkMirror) stop(moduleID string) {
	if u == nil {
		return
	}
	u.mu.Lock()
	if sub := u.subs[moduleID]; sub != nil {
		_ = sub.Unsubscribe()
		delete(u.subs, moduleID)
	}
	u.mu.Unlock()
}
