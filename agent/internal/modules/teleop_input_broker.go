package modules

import (
	"context"
	"fmt"
	"sync"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// teleopInputBroker mirrors the operator's generic gamepad snapshot
// (cmd.gamepad) into the sandboxed subtree of a module that declares
// requires=["teleop-input"], so the module can consume input without holding
// platform-subject permissions. Reverse of uplinkMirror.
//
// The mirror is gated on the rover operating mode: operator input is only
// forwarded in MODE_MANUAL. Outside manual (safe / autonomous / estop) the
// stream stops, and a consuming module's staleness failsafe holds its
// actuators. Modules can't observe event.mode themselves (sandbox), so this is
// the enforcement point.
type teleopInputBroker struct {
	nc      *natsgo.Conn
	roverID string

	mu      sync.Mutex
	subs    map[string]*natsgo.Subscription
	modeSub *natsgo.Subscription
	mode    waypointv1.Mode // current rover mode; gamepad mirrored only in MODE_MANUAL
}

func newTeleopInputBroker(nc *natsgo.Conn, roverID string) *teleopInputBroker {
	return &teleopInputBroker{nc: nc, roverID: roverID, subs: map[string]*natsgo.Subscription{}}
}

func (b *teleopInputBroker) start(_ context.Context, moduleID string) error {
	if b == nil || b.nc == nil {
		return fmt.Errorf("teleop-input broker: nil nats conn")
	}
	if err := b.ensureModeSub(); err != nil {
		return err
	}
	src := fmt.Sprintf("waypoint.%s.cmd.gamepad", b.roverID)
	dst := fmt.Sprintf("waypoint.%s.module.%s.input", b.roverID, moduleID)
	sub, err := b.nc.Subscribe(src, func(m *natsgo.Msg) {
		if err := proto.Unmarshal(m.Data, &waypointv1.GamepadSnapshot{}); err != nil {
			return // drop anything that isn't a GamepadSnapshot
		}
		b.mu.Lock()
		manual := b.mode == waypointv1.Mode_MODE_MANUAL
		b.mu.Unlock()
		if !manual {
			return // mode gate: forward operator input only in manual mode
		}
		_ = b.nc.Publish(dst, m.Data)
	})
	if err != nil {
		return fmt.Errorf("teleop-input broker: subscribe %s: %w", src, err)
	}
	b.mu.Lock()
	if old := b.subs[moduleID]; old != nil {
		_ = old.Unsubscribe()
	}
	b.subs[moduleID] = sub
	b.mu.Unlock()
	return nil
}

// ensureModeSub subscribes once to event.mode so the broker tracks the rover's
// operating mode. Core re-announces the current mode every ~1s, so a broker
// that starts mid-session learns the true mode within a second; until then the
// mode is unspecified and the gamepad mirror stays gated off (fail-safe).
func (b *teleopInputBroker) ensureModeSub() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.modeSub != nil {
		return nil
	}
	subject := fmt.Sprintf("waypoint.%s.event.mode", b.roverID)
	sub, err := b.nc.Subscribe(subject, func(m *natsgo.Msg) {
		var ev waypointv1.ModeEvent
		if proto.Unmarshal(m.Data, &ev) != nil {
			return
		}
		b.mu.Lock()
		b.mode = ev.GetTo()
		b.mu.Unlock()
	})
	if err != nil {
		return fmt.Errorf("teleop-input broker: subscribe event.mode: %w", err)
	}
	b.modeSub = sub
	return nil
}

func (b *teleopInputBroker) stop(moduleID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if sub := b.subs[moduleID]; sub != nil {
		_ = sub.Unsubscribe()
		delete(b.subs, moduleID)
	}
	b.mu.Unlock()
}
