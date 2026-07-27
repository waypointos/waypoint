# Setup: module SDK (`wpmodule`)

Author-facing guide for writing a Waypoint module in Go with the SDK. The SDK
(`github.com/waypointos/waypoint/sdk`, package `wpmodule`) absorbs the
runtime boilerplate every module needs (creds, connect, health, stats,
shutdown) and gives you typed clients for the agent's broker capabilities and
servers for the standard component APIs.

## What a module is

A module is a self-contained binary that runs alongside the agent and core on
the rover, talking to the rest of the platform over NATS inside its own subject
sandbox (`waypoint.<rover>.module.<id>.>`). The packaging, signing, and install
pipeline (the `.raw` portable image, cosign keyless signing, the registry, and
the on-rover reconciler) is covered in `no-rebuild-modules.md`. This guide
covers the code: how to write the module itself.

## Quick start

Copy `sdk/examples/sensor-minimal/` into a new repo. It is the smallest
complete module: a sensor component publishing two readings, one of them N/A.

`main.go`:

```go
// Command sensor-minimal is the smallest complete Waypoint module: a sensor
// component publishing two readings, one of them N/A, on the standard API.
package main

import (
	"context"
	"log/slog"
	"math"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sdk/wpmodule"
)

type demoSensor struct{ start time.Time }

func (d demoSensor) State() *waypointv1.SensorReadings {
	v := 12.0 + 0.3*math.Sin(time.Since(d.start).Seconds())
	return &waypointv1.SensorReadings{Readings: []*waypointv1.SensorReading{
		{Name: "bus_voltage", Value: proto.Float64(v), Unit: "V", Ok: true},
		{Name: "water_depth", Unit: "m", Ok: false}, // N/A: sensor not fitted
	}}
}

func main() {
	err := wpmodule.Run(context.Background(), wpmodule.Options{ID: "sensor-minimal"}, func(m *wpmodule.M) error {
		_, err := m.ServeSensor(demoSensor{start: time.Now()})
		return err
	})
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
```

`wpmodule.Run` connects, serves `module.<id>.health.ready`, publishes
`module.<id>.stats` heartbeats, runs your `setup` callback, sends sd_notify
READY, then blocks until the context is cancelled or SIGINT/SIGTERM arrives,
draining the connection on the way out. Your `setup` registers component servers
and capability clients and returns; the SDK owns the lifecycle.

### Dev loop

Run a dev rover in one terminal, then run your module against it in another:

```sh
make dev-rover PLATFORM=bench        # terminal 1: agent + core (sim)
make dev-module MODULE=./sdk/examples/sensor-minimal MODULE_ID=sensor-minimal
```

`make dev-module` builds the package and runs it with the standard env contract
pointed at the local dev rover. Dev NATS is open (no creds minted off-rover), so
the module connects as the default user. Pass extra env with
`MODULE_ENV='K=V ...'`.

## The env contract

The module units set these on the rover; the dev harness and `make dev-module`
set them locally. The SDK reads them so you do not parse flags or env yourself.

| Variable | Set by (on-rover) | Set by (dev) | Meaning |
|---|---|---|---|
| `WAYPOINT_ROVER_ID` | module unit | harness / `make dev-module` | rover id, the `<rover>` in every subject (required) |
| `WAYPOINT_NATS_URL` | module unit | harness / `make dev-module` | NATS URL; defaults to `nats://127.0.0.1:4222` when unset |
| `WAYPOINT_MODULE_CREDS` | module unit (creds.env path) | unset (plain connect) | path to creds.env (`WAYPOINT_NATS_USER` / `WAYPOINT_NATS_PASSWORD`); when unset the SDK connects plain |
| `WAYPOINT_MODULE_CONFIG` | module unit (config.toml path) | optional | path to the module's config.toml; `wpmodule.LoadConfig` decodes it |
| `WAYPOINT_MODULE_COMPONENT` | agent drop-in (from `[component]`) | optional | the component class (`arm` \| `sensor` \| `base`) |
| `WAYPOINT_MODULE_STATE_RATE_HZ` | agent drop-in (from `[component]`) | optional | component `.state` publish rate; the SDK defaults to 10 Hz when unset |
| `WAYPOINT_MODULE_ID` | dev harness / `make dev-module` | dev harness / `make dev-module` | overrides `Options.ID`; on-rover the id is compiled into the binary |

The component id is normally compiled into the binary through
`wpmodule.Options{ID: "..."}`. `WAYPOINT_MODULE_ID`, when set, overrides it,
which is how the harness runs an example binary under a chosen id.

## Component APIs

Declaring `[component]` in `module.toml` makes your module speak a standard
class. The SDK runs the state-publish loop at the manifest rate, stamps each
message, subscribes the command leaf, decodes, and dispatches to your
implementation. The agent auto-grants the class's standard leaves, so you do not
list them in `[permissions]`.

| Class | State leaf (publish) | Command leaf (subscribe) | SDK server | Interface |
|---|---|---|---|---|
| `arm` | `module.<id>.arm.state` | `module.<id>.arm.cmd` | `m.ServeArm(impl)` | `State() *ArmState`, `Command(*ArmCommand) error` |
| `sensor` | `module.<id>.sensor.state` | (none, read-only) | `m.ServeSensor(impl)` | `State() *SensorReadings` |
| `base` | `module.<id>.base.state` | `module.<id>.base.cmd` | `m.ServeBase(impl)` | `State() *BaseState`, `Command(*BaseCommand) error` |

`State()` is called at the configured rate; do not set the `Stamp` field, the
SDK fills it. `Command` receives a decoded message; for arms it is either
`ArmJointGoals` or a `stop` flag (joint-space only in v1). `stop` is a hard
contract: the implementation must halt any motion immediately and hold
position (re-latch goals to present, freeze any internal control loop); a
generic consumer relies on it. The conformance suite asserts this. Base is a reserved
contract: the shape is settled and SDK-served, but no production consumer exists
yet and core remains the platform's base.

These message types are in `protocol/messages/components.proto`, generated as
`waypointv1` Go bindings and `components_pb` TS bindings. The joint list IS the
arm schema: the `name` and order of `ArmJoint` entries define the arm for
consumers, so keep them stable. For sensors, an absent `value` with `ok = false`
is the N/A form; never publish a sentinel zero.

### Floor, not ceiling

The standard component API is a floor, not a ceiling. A module keeps its own
private subjects for anything the standard does not cover (calibration state, a
richer command proto, a panel feed). Those live under the same sandbox
(`module.<id>.<leaf>`) and are published with `m.Publish(m.Subject("..."))`.
The so100 arm, for example, serves the standard `arm.state` / `arm.cmd` while
keeping its private calibration and joint-angle subjects.

## Capability clients

Capabilities are the privileged platform access a module brokers through the
agent. Declare them in `module.toml`; the SDK gives you typed clients.

### servo-control (requires)

```toml
requires = ["servo-control"]
```

`m.Servo()` returns a `ServoClient` over the agent's servo-control broker
subjects (`module.<id>.servo.{cmd,sync,read}`). The agent relays to core's
generic servo surface and enforces the platform-owned deny-list (drive wheels
are refused); the module never holds `cmd.servo` or `rpc.servo_read` itself.
Methods: `SetMode`, `SetTorqueEnable`, `SetGoalPosition`, `SetTorqueLimit`,
`SetAngleLimits`, `SetOvercurrentLimit`, `SyncWriteGoals`, and `Read(id)` (a
request/reply round-trip returning `ServoState`). A `requires` capability only
attaches when the module is admin-registered through the signed-module trust
path.

### teleop-input (requires)

```toml
requires = ["teleop-input"]
```

`m.TeleopInput(cb)` subscribes the broker-relayed gamepad stream
(`module.<id>.input`) and calls `cb` with each decoded `GamepadSnapshot`.

### uplink (provides)

```toml
provides = ["uplink"]
```

`m.PublishUplink(u)` feeds the uplink rail (`module.<id>.uplink`); the agent
mirrors it onto `telemetry.uplink`. `provides` is an outbound, low-privilege
capability where the module feeds a platform rail.

### Escape hatch

`m.NC()` exposes the raw NATS connection. `m.Publish` / `m.Subscribe` /
`m.Request` are sandbox-scoped: they refuse any subject outside
`waypoint.<rover>.module.<id>.>`. Use `m.NC()` only to step outside
deliberately; the minted ACL on the server still applies.

## Proving your module

The conformance suite (`sim/conformance/module_test.go`) runs the real agent
and core (in sim) plus a module binary built from the SDK examples, and asserts
the component-class contracts end to end:

- `health.ready` responds with `ok`.
- `module.<id>.stats` flow and carry a capture stamp.
- the `.state` stream flows, is stamped, and runs at a plausible rate.
- commands act: arm conformance sends `ArmJointGoals` and observes
  `position_rad` converge while the underlying sim servo moves through the
  servo-control broker.
- estop freezes the component: a command during estop produces no servo motion
  and no goal change (the platform guarantee extended through the broker and the
  core gate to the component API).

The in-tree examples (`sensor-minimal`, `arm-sim`) keep the main-repo suite
self-contained. To prove your own out-of-tree module, run it against a dev rover
with the documented recipe in your module repo: `make dev-rover PLATFORM=bench`
in the waypoint repo, then run your module with the dev env vars and confirm
`module.<id>.<class>.state` traffic on the dashboard Bus pane. Your module's own
correctness is its unit tests plus this manual loop; the standard surface itself
is conformance-checked by the waypoint suite via the example modules.

To run the main-repo suite:

```sh
cmake -S core -B core/build -DWP_CORE_BUILD_TESTS=OFF && cmake --build core/build -j
cd agent && go build -o ../bin/waypoint-agent ./cmd/waypoint-agent && cd ..
cd sim && go test ./conformance/ -count=1 -timeout 900s
```

## Packaging

Packaging, signing, and installing the module are unchanged by the SDK. Build
the binary into a `.raw` portable image, cosign-sign it keyless in CI, register
it on the proxy, and enable it per rover. See `no-rebuild-modules.md` for the
full pipeline and the `tools/module-template/` skeleton.
