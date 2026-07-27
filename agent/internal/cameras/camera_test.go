package cameras

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/waypointos/waypoint/agent/internal/cameras/pipeline"
)

// flakyPipeline emits no frames and "dies" immediately on every Start, so
// supervise must keep restarting it.
type flakyPipeline struct {
	mu     sync.Mutex
	starts int
}

func (f *flakyPipeline) Name() string { return "flaky" }

func (f *flakyPipeline) Start(context.Context) (<-chan []byte, error) {
	f.mu.Lock()
	f.starts++
	f.mu.Unlock()
	ch := make(chan []byte)
	close(ch) // pipeline exits immediately
	return ch, nil
}

func (f *flakyPipeline) Stop() {}

func (f *flakyPipeline) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts
}

func TestCamera_RestartsPipelineOnExit(t *testing.T) {
	origMin, origMax := pipelineRestartMin, pipelineRestartMax
	pipelineRestartMin, pipelineRestartMax = 2*time.Millisecond, 5*time.Millisecond
	defer func() { pipelineRestartMin, pipelineRestartMax = origMin, origMax }()

	fp := &flakyPipeline{}
	cam, err := New("cam-0", fp, func() []webrtc.ICEServer { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := cam.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer cam.Stop()

	deadline := time.After(2 * time.Second)
	for fp.startCount() < 3 {
		select {
		case <-deadline:
			t.Fatalf("pipeline restarted only %d times; expected >=3", fp.startCount())
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func TestCamera_SubscriberReceivesFrames(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p := pipeline.NewSynthetic(pipeline.Config{Width: 320, Height: 240, FPS: 15, BitrateBps: 200_000})
	cam, err := New("chassis-front", p, func() []webrtc.ICEServer { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := cam.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer cam.Stop()

	subPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer subPC.Close()

	gotPkt := make(chan struct{}, 1)
	subPC.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		for {
			if _, _, err := track.Read(buf); err != nil {
				return
			}
			select {
			case gotPkt <- struct{}{}:
			default:
			}
			return
		}
	})

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

	answer, err := cam.NewSubscriber(ctx, "test-session", *subPC.LocalDescription())
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	if err := subPC.SetRemoteDescription(*answer); err != nil {
		t.Fatal(err)
	}

	select {
	case <-gotPkt:
	case <-time.After(8 * time.Second):
		t.Fatal("no RTP packet within 8s")
	}
}
