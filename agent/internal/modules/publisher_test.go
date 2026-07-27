package modules

import (
	"context"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestPublisher_EmitsModuleUI(t *testing.T) {
	specs := []ModuleSpec{{Manifest: Manifest{
		Name: "umr", Label: "Connectivity", Version: "0.1.0",
		UI: UIBinding{Kind: UIKindStatic, TabID: "m-umr"},
	}}}
	p := NewSnapshotPublisher(nil, "rover-1", specs)
	msg := p.buildSnapshot()
	require.Len(t, msg.Modules, 1)
	require.NotNil(t, msg.Modules[0].Ui)
	require.Equal(t, waypointv1.ModuleUI_KIND_STATIC, msg.Modules[0].Ui.Kind)
	require.Equal(t, "m-umr", msg.Modules[0].Ui.TabId)
	require.Equal(t, "m-umr", msg.Modules[0].TabId) // legacy field still filled
}

func TestSnapshotPublisher_PublishesAtInterval(t *testing.T) {
	srv, err := nats.StartEmbedded(nats.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	cli, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	var mu sync.Mutex
	var received []*waypointv1.ModuleSnapshot
	if _, err := cli.Subscribe("waypoint.r1.infra.modules", func(m *natsgo.Msg) {
		s := &waypointv1.ModuleSnapshot{}
		_ = proto.Unmarshal(m.Data, s)
		mu.Lock()
		received = append(received, s)
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}

	pub := NewSnapshotPublisher(cli, "r1", []ModuleSpec{
		{Manifest: Manifest{Name: "umr", Label: "Connectivity", Version: "0.1", UI: UIBinding{TabID: "m-umr"}}},
	})
	pub.SetInterval(50 * time.Millisecond)
	pub.SetHealth("umr", true)
	go pub.Run(ctx)

	time.Sleep(180 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(received) < 2 {
		t.Fatalf("want ≥2 snapshots, got %d", len(received))
	}
	last := received[len(received)-1]
	if len(last.Modules) != 1 || last.Modules[0].Id != "umr" || !last.Modules[0].Healthy {
		t.Fatalf("snapshot wrong: %+v", last.Modules)
	}
}

func TestPublisher_MergesRuntimeModules(t *testing.T) {
	p := NewSnapshotPublisher(nil, "rover-1", nil)
	p.SetRuntimeModule("power-monitor", "Power Monitor", "0.1.0",
		UIBinding{Kind: UIKindProxy, TabID: "m-power-monitor", ProxyPort: 8090, LANOnly: true},
		waypointv1.ModuleOrigin_MODULE_ORIGIN_UNSPECIFIED)
	p.SetHealth("power-monitor", true)

	msg := p.buildSnapshot()
	require.Len(t, msg.Modules, 1)
	require.Equal(t, "power-monitor", msg.Modules[0].Id)
	require.True(t, msg.Modules[0].Healthy)
	require.Equal(t, waypointv1.ModuleUI_KIND_PROXY, msg.Modules[0].Ui.Kind)

	p.RemoveRuntimeModule("power-monitor")
	require.Empty(t, p.buildSnapshot().Modules)
}

func TestPublisher_CarriesTeleopWindow(t *testing.T) {
	specs := []ModuleSpec{{Manifest: Manifest{
		Name: "so100", Label: "SO-100 Arm", Version: "0.1.0",
		UI: UIBinding{Kind: UIKindStatic, TabID: "m-so100", Teleop: &TeleopWindowBinding{
			WindowID: "w-so100", Label: "Arm", Entry: "arm.js", Bindings: []string{"RT=open"},
		}},
	}}}
	p := NewSnapshotPublisher(nil, "rover-1", specs)
	msg := p.buildSnapshot()
	require.Len(t, msg.Modules, 1)
	require.NotNil(t, msg.Modules[0].Ui)
	require.NotNil(t, msg.Modules[0].Ui.Teleop)
	require.Equal(t, "w-so100", msg.Modules[0].Ui.Teleop.WindowId)
	require.Equal(t, "arm.js", msg.Modules[0].Ui.Teleop.Entry)
	require.Equal(t, []string{"RT=open"}, msg.Modules[0].Ui.Teleop.Bindings)
}

// A teleop-only module (no static/proxy tab) must still carry its window so the
// dashboard host can render it.
func TestPublisher_TeleopOnlyNoTab(t *testing.T) {
	p := NewSnapshotPublisher(nil, "rover", nil)
	p.SetRuntimeModule("so100", "SO-100 Arm", "0.1.0", UIBinding{
		Kind: UIKindNone, Teleop: &TeleopWindowBinding{WindowID: "w-so100", Entry: "arm.js"},
	}, waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL)
	snap := p.buildSnapshot()
	require.Len(t, snap.Modules, 1)
	require.Empty(t, snap.Modules[0].TabId, "teleop-only module produces no tab")
	require.NotNil(t, snap.Modules[0].Ui)
	require.Equal(t, "w-so100", snap.Modules[0].Ui.Teleop.WindowId)
}

func TestPublisher_CarriesComponent(t *testing.T) {
	specs := []ModuleSpec{
		{Manifest: Manifest{
			Name: "myarm", Label: "My Arm", Version: "0.1.0",
			Component: &Component{Class: "arm", StateRateHz: 20},
		}},
		{Manifest: Manifest{
			Name: "plain", Label: "Plain", Version: "0.1.0",
		}},
	}
	p := NewSnapshotPublisher(nil, "rover-1", specs)
	msg := p.buildSnapshot()
	require.Len(t, msg.Modules, 2)
	byID := map[string]*waypointv1.ModuleInfo{}
	for _, info := range msg.Modules {
		byID[info.Id] = info
	}
	require.NotNil(t, byID["myarm"].GetComponent())
	require.Equal(t, "arm", byID["myarm"].GetComponent().GetClass())
	require.Equal(t, 20.0, byID["myarm"].GetComponent().GetStateRateHz())
	require.Nil(t, byID["plain"].GetComponent())
}

func TestSnapshot_RuntimeOrigin(t *testing.T) {
	p := NewSnapshotPublisher(nil, "rover", nil)
	p.SetRuntimeModule("umr", "UMR", "0.1.0", UIBinding{Kind: UIKindStatic, TabID: "m-umr"}, waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL)
	snap := p.buildSnapshot()
	require.Len(t, snap.Modules, 1)
	require.Equal(t, "umr", snap.Modules[0].Id)
	require.Equal(t, waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL, snap.Modules[0].Origin)
}

// Runtime modules live in a map, whose range order Go randomizes per pass. The
// dashboard derives tab order from this list, so it must be sorted by id.
func TestSnapshot_RuntimeOrderIsStable(t *testing.T) {
	p := NewSnapshotPublisher(nil, "rover", nil)
	for _, id := range []string{"umr", "so100", "power-monitor", "drill"} {
		p.SetRuntimeModule(id, id, "0.1.0", UIBinding{Kind: UIKindStatic, TabID: "m-" + id}, waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL)
	}
	want := []string{"drill", "power-monitor", "so100", "umr"}
	for i := 0; i < 50; i++ {
		var got []string
		for _, m := range p.buildSnapshot().Modules {
			got = append(got, m.Id)
		}
		require.Equal(t, want, got)
	}
}

// A module present both as a baked spec and a reconciler-attached runtime entry
// must surface once (runtime wins) so the dashboard shows a single tab.
func TestSnapshot_RuntimeShadowsBakedSpec(t *testing.T) {
	specs := []ModuleSpec{{Manifest: Manifest{
		Name: "umr", Label: "Baked", Version: "0.0.0",
		UI: UIBinding{Kind: UIKindStatic, TabID: "m-umr"},
	}}}
	p := NewSnapshotPublisher(nil, "rover", specs)
	p.SetRuntimeModule("umr", "Connectivity", "0.1.0", UIBinding{Kind: UIKindStatic, TabID: "m-umr"}, waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL)
	snap := p.buildSnapshot()
	require.Len(t, snap.Modules, 1)
	require.Equal(t, "Connectivity", snap.Modules[0].Label, "runtime entry should win")
	require.Equal(t, waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL, snap.Modules[0].Origin)
}
