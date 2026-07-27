package whep

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/waypointos/waypoint/agent/internal/cameras"
	"github.com/waypointos/waypoint/agent/internal/cameras/pipeline"
)

func TestPostOffer_ReturnsAnswerWith201(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 not installed")
	}

	p := pipeline.NewSynthetic(pipeline.Config{Width: 320, Height: 240, FPS: 15, BitrateBps: 200_000})
	cam, err := cameras.New("chassis-front", p, func() []webrtc.ICEServer { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := cam.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer cam.Stop()

	// Craft a valid SDP offer from a throwaway PC.
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
	offerSDP := subPC.LocalDescription().SDP

	mux := http.NewServeMux()
	_ = New(mux, Resolver{Cameras: func(name string) (*cameras.Camera, bool) {
		if name == "chassis-front" {
			return cam, true
		}
		return nil, false
	}})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/camera/chassis-front/whep", bytes.NewReader([]byte(offerSDP)))
	req.Header.Set("Content-Type", "application/sdp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "/camera/chassis-front/whep/") {
		t.Fatalf("bad Location: %q", loc)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "v=0") {
		t.Fatal("answer is not an SDP")
	}
}

func TestPostOffer_404OnMissingCamera(t *testing.T) {
	mux := http.NewServeMux()
	_ = New(mux, Resolver{Cameras: func(name string) (*cameras.Camera, bool) { return nil, false }})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/camera/nope/whep", "application/sdp", strings.NewReader("v=0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
