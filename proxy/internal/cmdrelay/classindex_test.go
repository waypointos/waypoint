package cmdrelay

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestParseComponentCmd(t *testing.T) {
	cases := []struct {
		subject            string
		rover, module, tok string
		ok                 bool
	}{
		{"waypoint.r1.module.drill.drill.cmd", "r1", "drill", "drill", true},
		{"waypoint.r1.module.drill.servo.cmd", "r1", "drill", "servo", true},
		{"waypoint.r1.module.so100.command", "", "", "", false},
		{"waypoint.r1.cmd.drive", "", "", "", false},
		{"waypoint.r1.module.drill.drill.state", "", "", "", false},
		{"waypoint.r1.module.drill.cmd", "", "", "", false},
		{"waypoint.r1.module.drill.a.b.cmd", "", "", "", false},
	}
	for _, c := range cases {
		rover, module, tok, ok := parseComponentCmd(c.subject)
		require.Equal(t, c.ok, ok, c.subject)
		if !c.ok {
			continue
		}
		require.Equal(t, c.rover, rover, c.subject)
		require.Equal(t, c.module, module, c.subject)
		require.Equal(t, c.tok, tok, c.subject)
	}
}

func snapshot(mods ...*waypointv1.ModuleInfo) []byte {
	b, err := proto.Marshal(&waypointv1.ModuleSnapshot{Modules: mods})
	if err != nil {
		panic(err)
	}
	return b
}

// The relay must forward a module's declared component command leaf and nothing
// else under module.<id>, so a browser cannot reach the agent's raw servo
// broker and bypass the module's interlocks.
func TestClassIndex_AllowsOnlyDeclaredClassLeaf(t *testing.T) {
	idx := newClassIndex()
	idx.learn("r1", snapshot(
		&waypointv1.ModuleInfo{Id: "drill", Component: &waypointv1.ModuleComponent{Class: "drill"}},
		&waypointv1.ModuleInfo{Id: "umr"}, // no [component]
	))

	require.True(t, idx.allow("waypoint.r1.module.drill.drill.cmd"))
	require.False(t, idx.allow("waypoint.r1.module.drill.servo.cmd"), "raw servo broker must never be relayed")
	require.False(t, idx.allow("waypoint.r1.module.umr.umr.cmd"), "module declaring no component has no cmd leaf")
	require.False(t, idx.allow("waypoint.r1.module.ghost.ghost.cmd"), "unknown module")
	require.False(t, idx.allow("waypoint.r2.module.drill.drill.cmd"), "class is learned per rover")
}

// A module serving several components must have every declared class leaf
// forwardable, not just the first one in manifest order.
func TestClassIndex_AllowsEveryDeclaredComponent(t *testing.T) {
	idx := newClassIndex()
	idx.learn("r1", snapshot(&waypointv1.ModuleInfo{
		Id:         "drill",
		Component:  &waypointv1.ModuleComponent{Class: "drill"},
		Components: []*waypointv1.ModuleComponent{{Class: "drill"}, {Class: "auger"}},
	}))

	require.True(t, idx.allow("waypoint.r1.module.drill.drill.cmd"))
	require.True(t, idx.allow("waypoint.r1.module.drill.auger.cmd"), "a non-first component leaf must relay too")
	require.False(t, idx.allow("waypoint.r1.module.drill.servo.cmd"), "raw servo broker must never be relayed")
}

// A class can change when a module is upgraded, and a module can disappear; the
// index must track the latest snapshot rather than accumulate stale grants.
func TestClassIndex_ReplacesPerRoverState(t *testing.T) {
	idx := newClassIndex()
	idx.learn("r1", snapshot(&waypointv1.ModuleInfo{Id: "drill", Component: &waypointv1.ModuleComponent{Class: "drill"}}))
	require.True(t, idx.allow("waypoint.r1.module.drill.drill.cmd"))

	idx.learn("r1", snapshot(&waypointv1.ModuleInfo{Id: "drill", Component: &waypointv1.ModuleComponent{Class: "auger"}}))
	require.True(t, idx.allow("waypoint.r1.module.drill.auger.cmd"))
	require.False(t, idx.allow("waypoint.r1.module.drill.drill.cmd"), "stale class must not linger")

	idx.learn("r1", snapshot())
	require.False(t, idx.allow("waypoint.r1.module.drill.auger.cmd"), "detached module loses its grant")
}

func TestClassIndex_IgnoresUndecodableSnapshot(t *testing.T) {
	idx := newClassIndex()
	idx.learn("r1", []byte("not a protobuf"))
	require.False(t, idx.allow("waypoint.r1.module.drill.drill.cmd"))
}
