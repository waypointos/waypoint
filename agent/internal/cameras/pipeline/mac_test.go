//go:build darwin

package pipeline

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMac_RealWebcam_Optional(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_MAC_CAM") != "1" {
		t.Skip("set WAYPOINT_TEST_MAC_CAM=1 to enable; will prompt for camera permission")
	}
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 not installed")
	}
	cfg := Config{Width: 640, Height: 360, FPS: 15, BitrateBps: 500_000}
	p := NewMac(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := p.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	deadline := time.After(4 * time.Second)
	for {
		select {
		case nal, ok := <-ch:
			if !ok {
				t.Fatal("closed without frames")
			}
			if nal[0]&0x1F == 5 { // IDR
				return
			}
		case <-deadline:
			t.Fatal("no IDR within 4s")
		}
	}
}
