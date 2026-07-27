package pipeline

import "context"

// Pipeline produces an Annex-B H.264 byte stream over an unbuffered channel.
// The returned channel is closed when Stop is called or the underlying source
// dies; the caller MUST drain promptly to avoid encoder back-pressure.
type Pipeline interface {
	Start(ctx context.Context) (<-chan []byte, error)
	Stop()
	Name() string
}

type Config struct {
	Width      int    // 1280
	Height     int    // 720
	FPS        int    // 30
	BitrateBps int    // 1_500_000 for production, 500_000 for synthetic
	Device     string // "" for synthetic, "/dev/video0" for Pi, "0" for Mac (avf index)
}

func DefaultConfig() Config {
	return Config{Width: 1280, Height: 720, FPS: 30, BitrateBps: 1_500_000}
}
