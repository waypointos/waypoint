package pipeline

import (
	"fmt"
	"runtime"
)

type Spec struct {
	Name   string // "synthetic", "mac", "pi5", or ""
	Device string
}

func Pick(s Spec, cfg Config) (Pipeline, error) {
	cfg.Device = s.Device
	switch s.Name {
	case "synthetic":
		return NewSynthetic(cfg), nil
	case "mac":
		return newMacOrErr(cfg)
	case "pi5":
		return newPi5OrErr(cfg)
	case "":
		// Auto-detect by OS.
		if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
			return newPi5OrErr(cfg)
		}
		if runtime.GOOS == "darwin" {
			return newMacOrErr(cfg)
		}
		return NewSynthetic(cfg), nil
	default:
		return nil, fmt.Errorf("unknown pipeline %q", s.Name)
	}
}

func errNotSupported(want, on string) error {
	return fmt.Errorf("pipeline %s not supported on %s", want, on)
}
