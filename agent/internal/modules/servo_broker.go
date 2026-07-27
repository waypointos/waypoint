package modules

import (
	"context"
	"fmt"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// servoBroker bridges a module's sandboxed servo subjects to core's platform
// servo surface, for modules that declare requires = ["servo-control"]. The
// module never holds cmd.servo / rpc.servo_read permissions; the agent (the sole
// writer of cmd.*) relays on its behalf. Mirrors uplinkMirror's lifecycle.
type servoBroker struct {
	nc      *natsgo.Conn
	roverID string

	// deny lists platform-owned bus ids the broker refuses to relay so a
	// module can never command core's wheels. Sourced from the descriptor.
	deny map[uint32]struct{}

	mu   sync.Mutex
	subs map[string][]*natsgo.Subscription
}

func newServoBroker(nc *natsgo.Conn, roverID string, deny map[uint32]struct{}) *servoBroker {
	return &servoBroker{nc: nc, roverID: roverID, deny: deny, subs: map[string][]*natsgo.Subscription{}}
}

func (b *servoBroker) start(_ context.Context, moduleID string) error {
	if b == nil || b.nc == nil {
		return fmt.Errorf("servo broker: nil nats conn")
	}
	cmdSrc := fmt.Sprintf("waypoint.%s.module.%s.servo.cmd", b.roverID, moduleID)
	cmdDst := fmt.Sprintf("waypoint.%s.cmd.servo", b.roverID)
	readSrc := fmt.Sprintf("waypoint.%s.module.%s.servo.read", b.roverID, moduleID)
	readDst := fmt.Sprintf("waypoint.%s.rpc.servo_read", b.roverID)

	cmdSub, err := b.nc.Subscribe(cmdSrc, func(m *natsgo.Msg) {
		var c waypointv1.ServoControl
		if err := proto.Unmarshal(m.Data, &c); err != nil {
			return // drop anything that isn't a ServoControl
		}
		if _, isDrive := b.deny[c.GetServoId()]; isDrive {
			return // drive-wheel guard
		}
		_ = b.nc.Publish(cmdDst, m.Data)
	})
	if err != nil {
		return fmt.Errorf("servo broker: subscribe %s: %w", cmdSrc, err)
	}

	readSub, err := b.nc.Subscribe(readSrc, func(m *natsgo.Msg) {
		if m.Reply == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := b.nc.RequestWithContext(ctx, readDst, m.Data)
		if err != nil {
			return
		}
		_ = m.Respond(resp.Data)
	})
	if err != nil {
		_ = cmdSub.Unsubscribe()
		return fmt.Errorf("servo broker: subscribe %s: %w", readSrc, err)
	}

	syncSrc := fmt.Sprintf("waypoint.%s.module.%s.servo.sync", b.roverID, moduleID)
	syncDst := fmt.Sprintf("waypoint.%s.cmd.servo_sync", b.roverID)
	syncSub, err := b.nc.Subscribe(syncSrc, func(m *natsgo.Msg) {
		var s waypointv1.ServoSyncWrite
		if err := proto.Unmarshal(m.Data, &s); err != nil {
			return
		}
		filtered := &waypointv1.ServoSyncWrite{}
		for _, g := range s.GetGoals() {
			if _, isDrive := b.deny[g.GetServoId()]; isDrive {
				continue // drive-wheel guard
			}
			filtered.Goals = append(filtered.Goals, g)
		}
		if len(filtered.Goals) == 0 {
			return
		}
		out, err := proto.Marshal(filtered)
		if err != nil {
			return
		}
		_ = b.nc.Publish(syncDst, out)
	})
	if err != nil {
		_ = cmdSub.Unsubscribe()
		_ = readSub.Unsubscribe()
		return fmt.Errorf("servo broker: subscribe %s: %w", syncSrc, err)
	}

	b.mu.Lock()
	for _, s := range b.subs[moduleID] {
		_ = s.Unsubscribe()
	}
	b.subs[moduleID] = []*natsgo.Subscription{cmdSub, readSub, syncSub}
	b.mu.Unlock()
	return nil
}

// startDev brokers servo subjects for ANY module id via wildcards. Dev images
// launch modules out-of-band (make dev-module, the sim conformance harness), so
// no per-module attach fires; this dev-only path keeps servo-control working
// for them. The deny-list still guards platform-owned bus ids.
func (b *servoBroker) startDev(_ context.Context) error {
	if b == nil || b.nc == nil {
		return fmt.Errorf("servo broker: nil nats conn")
	}
	cmdSrc := fmt.Sprintf("waypoint.%s.module.*.servo.cmd", b.roverID)
	cmdDst := fmt.Sprintf("waypoint.%s.cmd.servo", b.roverID)
	readSrc := fmt.Sprintf("waypoint.%s.module.*.servo.read", b.roverID)
	readDst := fmt.Sprintf("waypoint.%s.rpc.servo_read", b.roverID)
	syncSrc := fmt.Sprintf("waypoint.%s.module.*.servo.sync", b.roverID)
	syncDst := fmt.Sprintf("waypoint.%s.cmd.servo_sync", b.roverID)

	cmdSub, err := b.nc.Subscribe(cmdSrc, func(m *natsgo.Msg) {
		var c waypointv1.ServoControl
		if err := proto.Unmarshal(m.Data, &c); err != nil {
			return
		}
		if _, isDrive := b.deny[c.GetServoId()]; isDrive {
			return
		}
		_ = b.nc.Publish(cmdDst, m.Data)
	})
	if err != nil {
		return fmt.Errorf("servo broker: subscribe %s: %w", cmdSrc, err)
	}
	readSub, err := b.nc.Subscribe(readSrc, func(m *natsgo.Msg) {
		if m.Reply == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := b.nc.RequestWithContext(ctx, readDst, m.Data)
		if err != nil {
			return
		}
		_ = m.Respond(resp.Data)
	})
	if err != nil {
		_ = cmdSub.Unsubscribe()
		return fmt.Errorf("servo broker: subscribe %s: %w", readSrc, err)
	}
	syncSub, err := b.nc.Subscribe(syncSrc, func(m *natsgo.Msg) {
		var s waypointv1.ServoSyncWrite
		if err := proto.Unmarshal(m.Data, &s); err != nil {
			return
		}
		filtered := &waypointv1.ServoSyncWrite{}
		for _, g := range s.GetGoals() {
			if _, isDrive := b.deny[g.GetServoId()]; isDrive {
				continue
			}
			filtered.Goals = append(filtered.Goals, g)
		}
		if len(filtered.Goals) == 0 {
			return
		}
		out, err := proto.Marshal(filtered)
		if err != nil {
			return
		}
		_ = b.nc.Publish(syncDst, out)
	})
	if err != nil {
		_ = cmdSub.Unsubscribe()
		_ = readSub.Unsubscribe()
		return fmt.Errorf("servo broker: subscribe %s: %w", syncSrc, err)
	}

	b.mu.Lock()
	b.subs["*dev"] = []*natsgo.Subscription{cmdSub, readSub, syncSub}
	b.mu.Unlock()
	return nil
}

func (b *servoBroker) stop(moduleID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	for _, s := range b.subs[moduleID] {
		_ = s.Unsubscribe()
	}
	delete(b.subs, moduleID)
	b.mu.Unlock()
}
