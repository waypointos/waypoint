package sysinfo

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// fakeProbes lets tests dictate exactly what each probe returns on each call.
// Nil pointers mean "report not available".
type fakeProbes struct {
	cpu      *float64
	temp     *float64
	memUsed  *uint64
	memTotal *uint64
	uptime   *uint64
}

func (f fakeProbes) CPUPercent() (float64, bool) {
	if f.cpu == nil {
		return 0, false
	}
	return *f.cpu, true
}

func (f fakeProbes) TempC() (float64, bool) {
	if f.temp == nil {
		return 0, false
	}
	return *f.temp, true
}

func (f fakeProbes) MemoryBytes() (uint64, uint64, bool) {
	if f.memUsed == nil || f.memTotal == nil {
		return 0, 0, false
	}
	return *f.memUsed, *f.memTotal, true
}

func (f fakeProbes) UptimeSeconds() (uint64, bool) {
	if f.uptime == nil {
		return 0, false
	}
	return *f.uptime, true
}

// embeddedNATS starts an in-process NATS server, returns the client URL
// and registers cleanup. Same pattern as agent/internal/image/publisher_test.go.
func embeddedNATS(t *testing.T) (*nats.Conn, string) {
	t.Helper()
	s, err := server.NewServer(&server.Options{Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	t.Cleanup(func() { s.Shutdown(); s.WaitForShutdown() })
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc, s.ClientURL()
}

func ptr(f float64) *float64 { return &f }

func TestPublisher_emitsAtInterval(t *testing.T) {
	nc, _ := embeddedNATS(t)

	cpu, temp := 12.5, 45.0
	probes := fakeProbes{cpu: &cpu, temp: &temp}
	image := func() ImageInfo { return ImageInfo{Version: "v0.4.1", Partition: "B"} }

	p := New(nc, "rover-dev", probes, image, nil)
	p.interval = 10 * time.Millisecond // tests run faster than 1 Hz

	var (
		mu     sync.Mutex
		got    []*waypointv1.SystemTelemetry
		closed atomic.Bool
	)
	sub, err := nc.Subscribe("waypoint.rover-dev.telemetry.system", func(m *nats.Msg) {
		if closed.Load() {
			return
		}
		var msg waypointv1.SystemTelemetry
		if err := proto.Unmarshal(m.Data, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		mu.Lock()
		got = append(got, &msg)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	time.Sleep(45 * time.Millisecond)
	closed.Store(true)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 3 {
		t.Fatalf("expected >=3 messages in 45ms at 10ms interval; got %d", len(got))
	}
	m := got[0]
	if m.GetCpuPercent() != 12.5 {
		t.Errorf("cpu_percent = %v; want 12.5", m.GetCpuPercent())
	}
	if m.GetTemperatureC() != 45.0 {
		t.Errorf("temperature_c = %v; want 45.0", m.GetTemperatureC())
	}
	if m.GetImageVersion() != "v0.4.1" {
		t.Errorf("image_version = %q; want v0.4.1", m.GetImageVersion())
	}
	if m.GetImagePartition() != "B" {
		t.Errorf("image_partition = %q; want B", m.GetImagePartition())
	}
	if m.GetT() == nil {
		t.Error("t (timestamp) is nil; want set")
	}
	if m.GetStamp() == nil {
		t.Fatal("stamp is nil; want capture-time dual stamp")
	}
	if m.GetStamp().GetMonoNs() == 0 {
		t.Error("stamp.mono_ns = 0; want nonzero monotonic capture")
	}
}

func TestPublisher_omitsFieldsWhenProbesAreFalse(t *testing.T) {
	nc, _ := embeddedNATS(t)

	probes := fakeProbes{} // both pointers nil -> CPUPercent/TempC return false
	image := func() ImageInfo { return ImageInfo{} } // empty -> both fields omitted

	p := New(nc, "rover-dev", probes, image, nil)
	p.interval = 10 * time.Millisecond

	gotCh := make(chan *waypointv1.SystemTelemetry, 4)
	_, err := nc.Subscribe("waypoint.rover-dev.telemetry.system", func(m *nats.Msg) {
		var msg waypointv1.SystemTelemetry
		if err := proto.Unmarshal(m.Data, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		select {
		case gotCh <- &msg:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	select {
	case msg := <-gotCh:
		if msg.CpuPercent != nil {
			t.Errorf("cpu_percent should be absent; got %v", *msg.CpuPercent)
		}
		if msg.TemperatureC != nil {
			t.Errorf("temperature_c should be absent; got %v", *msg.TemperatureC)
		}
		if msg.ImageVersion != nil {
			t.Errorf("image_version should be absent; got %q", *msg.ImageVersion)
		}
		if msg.ImagePartition != nil {
			t.Errorf("image_partition should be absent; got %q", *msg.ImagePartition)
		}
		if msg.T == nil {
			t.Error("t (timestamp) is nil; want set even when all other fields absent")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no SystemTelemetry received within 100ms")
	}
}

func TestPublisher_emitsLinkRttWhenRttFnReports(t *testing.T) {
	nc, _ := embeddedNATS(t)

	p := New(nc, "rover-dev", fakeProbes{}, func() ImageInfo { return ImageInfo{} },
		func() (float64, bool) { return 12.5, true })
	p.interval = 10 * time.Millisecond

	gotCh := make(chan *waypointv1.SystemTelemetry, 4)
	_, err := nc.Subscribe("waypoint.rover-dev.telemetry.system", func(m *nats.Msg) {
		var msg waypointv1.SystemTelemetry
		if err := proto.Unmarshal(m.Data, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		select {
		case gotCh <- &msg:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	select {
	case msg := <-gotCh:
		if msg.LinkRttMs == nil {
			t.Fatal("link_rtt_ms should be set when rtt fn reports a measurement")
		}
		if *msg.LinkRttMs != 12.5 {
			t.Fatalf("link_rtt_ms = %v, want 12.5", *msg.LinkRttMs)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no SystemTelemetry received within 100ms")
	}
}

func TestPublisher_omitsLinkRttWhenRttFnReportsFalse(t *testing.T) {
	nc, _ := embeddedNATS(t)

	p := New(nc, "rover-dev", fakeProbes{}, func() ImageInfo { return ImageInfo{} },
		func() (float64, bool) { return 0, false })
	p.interval = 10 * time.Millisecond

	gotCh := make(chan *waypointv1.SystemTelemetry, 4)
	_, err := nc.Subscribe("waypoint.rover-dev.telemetry.system", func(m *nats.Msg) {
		var msg waypointv1.SystemTelemetry
		if err := proto.Unmarshal(m.Data, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		select {
		case gotCh <- &msg:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	select {
	case msg := <-gotCh:
		if msg.LinkRttMs != nil {
			t.Fatalf("link_rtt_ms should be absent when rtt fn reports unavailable; got %v", *msg.LinkRttMs)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no SystemTelemetry received within 100ms")
	}
}

func TestPublisher_ctxCancelStops(t *testing.T) {
	nc, _ := embeddedNATS(t)
	p := New(nc, "rover-dev", fakeProbes{}, func() ImageInfo { return ImageInfo{} }, nil)
	p.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	time.Sleep(15 * time.Millisecond) // let at least one tick happen
	cancel()

	select {
	case <-done:
		// Run returned within budget; success.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of ctx cancel")
	}
}
