package conformance

// Conformance suite: launches the REAL waypoint-agent and waypoint-core (the
// latter in sim mode) and drives them through NATS, asserting safety behaviors
// and a so100-style hard-stop seek with zero hardware. Each test runs in
// controlled-clock mode so virtual time advances only via rpc.sim_advance,
// making the traces deterministic.
//
// The suite is a platform matrix: every test runs over each descriptor in
// platforms(t), deriving expectations from the descriptor so a structurally
// different platform (an arm-only bench) exercises the same seam.

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/waypointos/waypoint/protocol/platform/descriptor"
	"github.com/waypointos/waypoint/sim/internal/harness"
)

// Binary paths. Defaults mirror waypoint-dev, resolved relative to this
// package's directory (sim/conformance), and are overridable via env so CI can
// point at prebuilt artifacts.
func agentBin() string { return envOr("WAYPOINT_AGENT_BIN", "../../bin/waypoint-agent") }
func coreBin() string  { return envOr("WAYPOINT_CORE_BIN", "../../core/build/src/waypoint-core") }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// platformUT is one platform under test: its descriptor file plus the parsed
// form every expectation derives from.
type platformUT struct {
	id   string
	path string
	desc *descriptor.Descriptor
}

func (p platformUT) hasDrive() bool { return p.desc.Kinematics != nil }

// driveWheelIDs derives the platform-owned drive set from kinematics.wheels.
func (p platformUT) driveWheelIDs(t *testing.T) map[uint32]bool {
	t.Helper()
	out := map[uint32]bool{}
	for _, name := range p.desc.Kinematics.Wheels {
		id, ok := p.desc.BusIDFor(name)
		require.True(t, ok, "wheel joint %s missing", name)
		out[id] = true
	}
	return out
}

// goldenWheelRadps is the per-wheel |omega| expectation for vx=1.0 m/s,
// derived from the descriptor (Go derivation vs C++ behavior).
func (p platformUT) goldenWheelRadps(t *testing.T) map[uint32]float64 {
	t.Helper()
	speeds, err := p.desc.WheelSpeeds(1.0, 0)
	require.NoError(t, err)
	out := map[uint32]float64{}
	for name, w := range speeds {
		id, ok := p.desc.BusIDFor(name)
		require.True(t, ok)
		out[id] = math.Abs(w)
	}
	return out
}

// seekServoID is the first declared revolute joint: the hard-stop seek target.
func (p platformUT) seekServoID(t *testing.T) uint32 {
	t.Helper()
	for _, j := range p.desc.Joints {
		if j.Type == "revolute" {
			return j.BusID
		}
	}
	t.Fatal("platform declares no revolute joint")
	return 0
}

// moduleWheelIDs is the module-owned wheel set, the servos core sweeps to a
// stop on estop. Empty on a platform that declares none.
func (p platformUT) moduleWheelIDs() map[uint32]bool {
	out := map[uint32]bool{}
	for _, j := range p.desc.Joints {
		if j.Ownership == "module" && j.Type == "wheel" {
			out[j.BusID] = true
		}
	}
	return out
}

// absentServoID is the last declared joint: the absence scenario target.
func (p platformUT) absentServoID() uint32 {
	return p.desc.Joints[len(p.desc.Joints)-1].BusID
}

// platforms returns the matrix under test. WAYPOINT_DESCRIPTOR pins it to a
// single explicit descriptor.
func platforms(t *testing.T) []platformUT {
	t.Helper()
	paths := []string{
		"../../protocol/platform/waypoint-rover.toml",
		"../../protocol/platform/waypoint-bench.toml",
	}
	if p := os.Getenv("WAYPOINT_DESCRIPTOR"); p != "" {
		paths = []string{p}
	}
	var out []platformUT
	for _, path := range paths {
		d, err := descriptor.Load(path)
		require.NoError(t, err, path)
		out = append(out, platformUT{id: d.Platform.ID, path: path, desc: d})
	}
	return out
}

// launch builds harness.Opts, skips the test if a real binary is missing (so
// `go test ./...` stays runnable without the C++ build), launches the rover,
// and registers cleanup. controlled selects sim-controlled virtual time.
func launch(t *testing.T, controlled bool, p platformUT) *harness.Rover {
	t.Helper()
	requireBinary(t, agentBin(), "WAYPOINT_AGENT_BIN")
	requireBinary(t, coreBin(), "WAYPOINT_CORE_BIN")
	requireBinary(t, p.path, "WAYPOINT_DESCRIPTOR")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, err := harness.Launch(ctx, harness.Opts{
		AgentBin:       agentBin(),
		CoreBin:        coreBin(),
		DescriptorPath: p.path,
		RoverID:        "sim-rover",
		Controlled:     controlled,
		HTTPAddr:       freeHTTPAddr(),
		LogDir:         t.TempDir(),
	})
	require.NoError(t, err, "harness launch (%s)", p.id)
	t.Cleanup(r.Stop)
	return r
}

func requireBinary(t *testing.T, path, env string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing %s (%s=%q); build it first: `make core` for waypoint-core, `cd agent && go build -o ../bin/waypoint-agent ./cmd/waypoint-agent` for the agent", env, env, path)
	}
}

// freeHTTPAddr returns an agent HTTP listen address. The conformance suite never
// touches the gateway; binding :0 avoids clashing with a dev rover on :8080.
func freeHTTPAddr() string { return "127.0.0.1:0" }
