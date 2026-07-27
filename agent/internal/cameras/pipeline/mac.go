//go:build darwin

package pipeline

import (
	"context"
	"fmt"
	"sync"
)

type Mac struct {
	cfg Config
	mu  sync.Mutex // Start runs on the camera supervise goroutine; Stop does not
	gp  *gstProc
}

func NewMac(cfg Config) *Mac { return &Mac{cfg: cfg} }

func (m *Mac) Name() string { return "mac" }

func (m *Mac) Start(ctx context.Context) (<-chan []byte, error) {
	devIdx := m.cfg.Device
	if devIdx == "" {
		devIdx = "0"
	}
	args := []string{
		"avfvideosrc", fmt.Sprintf("device-index=%s", devIdx),
		"!", "videoconvert",
		"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1", m.cfg.Width, m.cfg.Height, m.cfg.FPS),
		"!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast", "key-int-max=60",
		fmt.Sprintf("bitrate=%d", m.cfg.BitrateBps/1000),
		"!", "video/x-h264,stream-format=byte-stream,profile=baseline",
		"!", "h264parse", "config-interval=-1",
		"!", "fdsink", "fd=1",
	}
	ch, gp, err := startGst(ctx, args)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.gp = gp
	m.mu.Unlock()
	return ch, nil
}

func (m *Mac) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gp != nil {
		m.gp.Stop()
	}
}
