package pipeline

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestSynthetic_ProducesNALs(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 not installed; skipping")
	}

	cfg := Config{Width: 320, Height: 240, FPS: 15, BitrateBps: 200_000}
	p := NewSynthetic(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := p.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	gotSPS, gotPPS, gotFrame := false, false, false
	deadline := time.After(3 * time.Second)
	for !(gotSPS && gotPPS && gotFrame) {
		select {
		case nal, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before we saw SPS/PPS/frame")
			}
			if len(nal) == 0 {
				continue
			}
			switch nal[0] & 0x1F {
			case 7:
				gotSPS = true
			case 8:
				gotPPS = true
			case 1, 5:
				gotFrame = true
			}
		case <-deadline:
			t.Fatalf("timeout — SPS=%v PPS=%v frame=%v", gotSPS, gotPPS, gotFrame)
		}
	}
}
