package modules

import (
	"context"
	"fmt"
	"sync"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// driveBroker bridges a module's sandboxed drive subjects to core's platform
// drive surface, for modules that declare requires = ["drive-control"]. The
// module never holds cmd.drive / telemetry.drive permissions; the agent (the
// sole writer of cmd.*) relays on its behalf.
//
// The command relay is gated on the rover operating mode: module drive commands
// are forwarded only in MODE_AUTONOMOUS. Outside autonomous the relay drops
// silently and core's drive-staleness failsafe halts the base. Modules can't
// observe event.mode themselves (sandbox), so the broker also mirrors it to
// module.<id>.mode so a module knows when it holds drive authority.
type driveBroker struct {
	nc      *natsgo.Conn
	roverID string

	mu      sync.Mutex
	subs    map[string][]*natsgo.Subscription
	modeSub *natsgo.Subscription
	mode    waypointv1.Mode // current rover mode; commands relayed only in MODE_AUTONOMOUS
}

func newDriveBroker(nc *natsgo.Conn, roverID string) *driveBroker {
	return &driveBroker{nc: nc, roverID: roverID, subs: map[string][]*natsgo.Subscription{}}
}

func (b *driveBroker) start(ctx context.Context, moduleID string) error {
	if b == nil || b.nc == nil {
		return fmt.Errorf("drive broker: nil nats conn")
	}
	if err := b.ensureModeSub(); err != nil {
		return err
	}
	cmdSrc := fmt.Sprintf("waypoint.%s.module.%s.drive.cmd", b.roverID, moduleID)
	cmdDst := fmt.Sprintf("waypoint.%s.cmd.drive", b.roverID)
	cmdSub, err := b.nc.Subscribe(cmdSrc, b.relayCmd(cmdDst))
	if err != nil {
		return fmt.Errorf("drive broker: subscribe %s: %w", cmdSrc, err)
	}
	if err := b.startMirrors(ctx, moduleID, cmdSub); err != nil {
		_ = cmdSub.Unsubscribe()
		return err
	}
	return nil
}

// startMirrors starts only the platform→module legs (drive.telemetry and
// mode). On dev images the wildcard relay already covers drive.cmd for every
// module, but mirrors have a per-module destination and cannot be wildcarded,
// so an attached module still needs them started individually.
func (b *driveBroker) startMirrors(_ context.Context, moduleID string, extra ...*natsgo.Subscription) error {
	if b == nil || b.nc == nil {
		return fmt.Errorf("drive broker: nil nats conn")
	}
	if err := b.ensureModeSub(); err != nil {
		return err
	}
	telemSrc := fmt.Sprintf("waypoint.%s.telemetry.drive", b.roverID)
	telemDst := fmt.Sprintf("waypoint.%s.module.%s.drive.telemetry", b.roverID, moduleID)
	telemSub, err := b.nc.Subscribe(telemSrc, func(m *natsgo.Msg) {
		_ = b.nc.Publish(telemDst, m.Data)
	})
	if err != nil {
		return fmt.Errorf("drive broker: subscribe %s: %w", telemSrc, err)
	}

	modeSrc := fmt.Sprintf("waypoint.%s.event.mode", b.roverID)
	modeDst := fmt.Sprintf("waypoint.%s.module.%s.mode", b.roverID, moduleID)
	modeSub, err := b.nc.Subscribe(modeSrc, func(m *natsgo.Msg) {
		_ = b.nc.Publish(modeDst, m.Data)
	})
	if err != nil {
		_ = telemSub.Unsubscribe()
		return fmt.Errorf("drive broker: subscribe %s: %w", modeSrc, err)
	}

	b.mu.Lock()
	for _, s := range b.subs[moduleID] {
		_ = s.Unsubscribe()
	}
	b.subs[moduleID] = append(append([]*natsgo.Subscription{}, extra...), telemSub, modeSub)
	b.mu.Unlock()
	return nil
}

// startDev relays drive commands for ANY module id via a wildcard, matching
// servoBroker.startDev for out-of-band dev modules. Only the command leg is
// wildcardable; dev modules connect through the unnarrowed default user, so
// they read telemetry.drive and event.mode directly.
func (b *driveBroker) startDev(_ context.Context) error {
	if b == nil || b.nc == nil {
		return fmt.Errorf("drive broker: nil nats conn")
	}
	if err := b.ensureModeSub(); err != nil {
		return err
	}
	cmdSrc := fmt.Sprintf("waypoint.%s.module.*.drive.cmd", b.roverID)
	cmdDst := fmt.Sprintf("waypoint.%s.cmd.drive", b.roverID)
	cmdSub, err := b.nc.Subscribe(cmdSrc, b.relayCmd(cmdDst))
	if err != nil {
		return fmt.Errorf("drive broker: subscribe %s: %w", cmdSrc, err)
	}
	b.mu.Lock()
	b.subs["*dev"] = []*natsgo.Subscription{cmdSub}
	b.mu.Unlock()
	return nil
}

func (b *driveBroker) relayCmd(dst string) natsgo.MsgHandler {
	return func(m *natsgo.Msg) {
		if proto.Unmarshal(m.Data, &waypointv1.DriveCommand{}) != nil {
			return // drop anything that isn't a DriveCommand
		}
		b.mu.Lock()
		autonomous := b.mode == waypointv1.Mode_MODE_AUTONOMOUS
		b.mu.Unlock()
		if !autonomous {
			return // mode gate: modules hold drive authority only in autonomous
		}
		_ = b.nc.Publish(dst, m.Data)
	}
}

// ensureModeSub subscribes once to event.mode so the broker tracks the rover's
// operating mode. Core re-announces the current mode every ~1s, so a broker
// that starts mid-session learns the true mode within a second; until then the
// mode is unspecified and the command relay stays gated off (fail-safe).
func (b *driveBroker) ensureModeSub() error {
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
		return fmt.Errorf("drive broker: subscribe event.mode: %w", err)
	}
	b.modeSub = sub
	return nil
}

func (b *driveBroker) stop(moduleID string) {
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
