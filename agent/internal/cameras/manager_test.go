package cameras

import (
	"context"
	"os/exec"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestManager_PublishesCameraListOnStart(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 not installed")
	}

	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("waypoint.sim-01.infra.camera.list", func(msg *nats.Msg) {
		got <- msg.Data
	}); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(ManagerConfig{
		Cameras: []CameraSpec{{Name: "chassis-front", Pipeline: "synthetic"}},
		NC:      nc,
		RoverID: "sim-01",
		IceCfg:  func() []webrtc.ICEServer { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	select {
	case payload := <-got:
		if len(payload) == 0 {
			t.Fatal("empty camera.list payload")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no camera.list within 3s")
	}
}

func TestManager_HandlesCameraOfferRPC(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 not installed")
	}

	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	mgr := NewManager(ManagerConfig{
		Cameras: []CameraSpec{{Name: "chassis-front", Pipeline: "synthetic"}},
		NC:      nc,
		RoverID: "sim-01",
		IceCfg:  func() []webrtc.ICEServer { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	// Build a real SDP offer from a throwaway PC.
	subPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer subPC.Close()
	if _, err := subPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}
	offer, err := subPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := subPC.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-webrtc.GatheringCompletePromise(subPC)

	req := &waypointv1.CameraOfferRequest{
		Camera:    "chassis-front",
		SessionId: "test-sess",
		OfferSdp:  subPC.LocalDescription().SDP,
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := nc.Request("waypoint.sim-01.rpc.camera_offer", body, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var resp waypointv1.CameraOfferResponse
	if err := proto.Unmarshal(rep.Data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil && *resp.Error != "" {
		t.Fatalf("rpc error: %s", *resp.Error)
	}
	if resp.AnswerSdp == "" {
		t.Fatal("empty answer SDP")
	}
}

func TestManager_HotplugReconcile(t *testing.T) {
	// Keep failed pipelines from busy-retrying during the test: we only assert
	// the advertised set (m.cams membership), which needs no real frames, so
	// this test has no gst dependency.
	origMin := pipelineRestartMin
	pipelineRestartMin = time.Hour
	defer func() { pipelineRestartMin = origMin }()

	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	// Injectable discovery the test mutates to simulate plug/unplug.
	cam := func(n, dev string) CameraSpec {
		return CameraSpec{Name: n, Device: dev, Pipeline: "synthetic"}
	}
	var mu sync.Mutex
	devices := []CameraSpec{cam("camera-0", "/dev/video0")}
	mgr := NewManager(ManagerConfig{
		NC:      nc,
		RoverID: "sim-01",
		IceCfg:  func() []webrtc.ICEServer { return nil },
		Discover: func() []CameraSpec {
			mu.Lock()
			defer mu.Unlock()
			return append([]CameraSpec(nil), devices...)
		},
	})
	setDevices := func(d ...CameraSpec) { mu.Lock(); devices = d; mu.Unlock() }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	assertList := func(want ...string) {
		t.Helper()
		got := mgr.List()
		sort.Strings(got)
		sort.Strings(want)
		// Compare by length+elements so an empty result ([]string{}) matches a
		// no-arg want (nil) — reflect.DeepEqual treats those as unequal.
		equal := len(got) == len(want)
		for i := 0; equal && i < len(got); i++ {
			equal = got[i] == want[i]
		}
		if !equal {
			t.Fatalf("camera list = %v, want %v", got, want)
		}
	}

	assertList("camera-0") // boot set

	setDevices(cam("camera-0", "/dev/video0"), cam("camera-2", "/dev/video2")) // second appears
	mgr.reconcile(ctx)
	assertList("camera-0", "camera-2")

	setDevices(cam("camera-2", "/dev/video2")) // camera-0 unplugged
	mgr.reconcile(ctx)
	assertList("camera-2")

	setDevices() // all gone
	mgr.reconcile(ctx)
	assertList()
}

func TestManager_OfferRPC_UnknownCamera(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	mgr := NewManager(ManagerConfig{
		NC:      nc,
		RoverID: "sim-01",
		IceCfg:  func() []webrtc.ICEServer { return nil },
	})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	req := &waypointv1.CameraOfferRequest{Camera: "nope", SessionId: "s", OfferSdp: ""}
	body, _ := proto.Marshal(req)
	rep, err := nc.Request("waypoint.sim-01.rpc.camera_offer", body, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var resp waypointv1.CameraOfferResponse
	if err := proto.Unmarshal(rep.Data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || *resp.Error == "" {
		t.Fatal("expected error reply for unknown camera")
	}
}
