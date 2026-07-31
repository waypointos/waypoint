package led

import (
	"fmt"
	"log/slog"
)

// Driver pushes a rendered frame to the strip.
type Driver interface {
	Frame(pixels []RGB) error
	Close() error
}

// LogDriver stands in when no SPI device is available (sim mode, dev on
// macOS). It logs only on change to keep the output readable.
type LogDriver struct {
	last string
}

func (d *LogDriver) Frame(pixels []RGB) error {
	s := ""
	for _, p := range pixels {
		s += fmt.Sprintf("[%d,%d,%d]", p.R, p.G, p.B)
	}
	if s != d.last {
		d.last = s
		slog.Info("led: frame", "pixels", s)
	}
	return nil
}

func (d *LogDriver) Close() error { return nil }
