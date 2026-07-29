package wpmodule

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/protocol/platform/stamp"
)

// ArmServer is implemented by an arm module; the SDK owns the wire.
type ArmServer interface {
	State() *waypointv1.ArmState
	Command(cmd *waypointv1.ArmCommand) error
}

// SensorServer is read-only: no command path.
type SensorServer interface {
	State() *waypointv1.SensorReadings
}

// BaseServer is the reserved mobility contract; no production consumer yet.
type BaseServer interface {
	State() *waypointv1.BaseState
	Command(cmd *waypointv1.BaseCommand) error
}

// classEnvSuffix maps a component class to its env-var suffix: upper-case,
// "-" becomes "_". Mirrors the agent's drop-in naming.
func classEnvSuffix(class string) string {
	return strings.ToUpper(strings.ReplaceAll(class, "-", "_"))
}

// stateRateFor resolves a serve loop's rate: class-specific env var, then
// the global var, then 10 Hz. Invalid values fall through to the next source.
func stateRateFor(class string) time.Duration {
	hz := 10.0
	for _, key := range []string{
		"WAYPOINT_MODULE_STATE_RATE_HZ_" + classEnvSuffix(class),
		"WAYPOINT_MODULE_STATE_RATE_HZ",
	} {
		if v := os.Getenv(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 100 {
				hz = f
				break
			}
		}
	}
	return time.Duration(float64(time.Second) / hz)
}

// serveState runs the publish loop; build returns the marshaled, stamped
// state. The returned stop is idempotent.
func (m *M) serveState(leaf string, rate time.Duration, build func() ([]byte, error)) func() {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(rate)
		defer t.Stop()
		subj := m.Subject(leaf)
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if b, err := build(); err == nil {
					_ = m.nc.Publish(subj, b)
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stop) }) }
}

// register makes Run own a component server's lifecycle: the server stops at
// shutdown (before drain) even when the caller discards the stop func.
func (m *M) register(stopLoop func(), sub *natsgo.Subscription) func() {
	var once sync.Once
	stop := func() {
		once.Do(func() {
			stopLoop()
			if sub != nil {
				_ = sub.Unsubscribe()
			}
		})
	}
	m.addStop(stop)
	return stop
}

func (m *M) ServeArm(srv ArmServer) (stop func(), err error) {
	sub, err := m.Subscribe(m.Subject("arm.cmd"), func(msg *natsgo.Msg) {
		var cmd waypointv1.ArmCommand
		if proto.Unmarshal(msg.Data, &cmd) != nil {
			return
		}
		_ = srv.Command(&cmd)
	})
	if err != nil {
		return nil, err
	}
	stopLoop := m.serveState("arm.state", stateRateFor("arm"), func() ([]byte, error) {
		st := srv.State()
		st.Stamp = stamp.Now()
		return proto.Marshal(st)
	})
	return m.register(stopLoop, sub), nil
}

func (m *M) ServeSensor(srv SensorServer) (stop func(), err error) {
	stopLoop := m.serveState("sensor.state", stateRateFor("sensor"), func() ([]byte, error) {
		st := srv.State()
		st.Stamp = stamp.Now()
		return proto.Marshal(st)
	})
	return m.register(stopLoop, nil), nil
}

func (m *M) ServeBase(srv BaseServer) (stop func(), err error) {
	sub, err := m.Subscribe(m.Subject("base.cmd"), func(msg *natsgo.Msg) {
		var cmd waypointv1.BaseCommand
		if proto.Unmarshal(msg.Data, &cmd) != nil {
			return
		}
		_ = srv.Command(&cmd)
	})
	if err != nil {
		return nil, err
	}
	stopLoop := m.serveState("base.state", stateRateFor("base"), func() ([]byte, error) {
		st := srv.State()
		st.Stamp = stamp.Now()
		return proto.Marshal(st)
	})
	return m.register(stopLoop, sub), nil
}
