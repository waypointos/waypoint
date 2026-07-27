//go:build linux && arm64

package pipeline

import (
	"context"
	"fmt"
	"sync"
)

type Pi5 struct {
	cfg Config
	mu  sync.Mutex // Start runs on the camera supervise goroutine; Stop does not
	gp  *gstProc
}

func NewPi5(cfg Config) *Pi5 { return &Pi5{cfg: cfg} }

func (p *Pi5) Name() string { return "pi5" }

func (p *Pi5) Start(ctx context.Context) (<-chan []byte, error) {
	dev := p.cfg.Device
	if dev == "" {
		dev = "/dev/video0"
	}
	bitrate := p.cfg.BitrateBps
	if bitrate == 0 {
		bitrate = 1_500_000
	}
	args := []string{
		"v4l2src", fmt.Sprintf("device=%s", dev),
		"!", fmt.Sprintf("image/jpeg,width=%d,height=%d,framerate=%d/1", p.cfg.Width, p.cfg.Height, p.cfg.FPS),
		"!", "jpegdec",
		"!", "videoconvert",
		// Pi 5 has no hardware H.264 encoder; encode in software with x264.
		"!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast", "key-int-max=60",
		fmt.Sprintf("bitrate=%d", bitrate/1000),
		"!", "video/x-h264,stream-format=byte-stream,profile=baseline",
		"!", "h264parse", "config-interval=-1",
		"!", "fdsink", "fd=1",
	}
	ch, gp, err := startGst(ctx, args)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.gp = gp
	p.mu.Unlock()
	return ch, nil
}

func (p *Pi5) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gp != nil {
		p.gp.Stop()
	}
}
