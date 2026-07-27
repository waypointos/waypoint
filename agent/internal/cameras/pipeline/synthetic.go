package pipeline

import (
	"context"
	"fmt"
	"sync"
)

type Synthetic struct {
	cfg Config
	mu  sync.Mutex // Start runs on the camera supervise goroutine; Stop does not
	gp  *gstProc
}

func NewSynthetic(cfg Config) *Synthetic { return &Synthetic{cfg: cfg} }

func (s *Synthetic) Name() string { return "synthetic" }

func (s *Synthetic) Start(ctx context.Context) (<-chan []byte, error) {
	args := []string{
		"videotestsrc", "is-live=true",
		"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1", s.cfg.Width, s.cfg.Height, s.cfg.FPS),
		"!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast", "key-int-max=60",
		fmt.Sprintf("bitrate=%d", s.cfg.BitrateBps/1000),
		"!", "video/x-h264,stream-format=byte-stream,profile=baseline",
		"!", "h264parse", "config-interval=-1",
		"!", "fdsink", "fd=1",
	}
	ch, gp, err := startGst(ctx, args)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.gp = gp
	s.mu.Unlock()
	return ch, nil
}

func (s *Synthetic) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gp != nil {
		s.gp.Stop()
	}
}
