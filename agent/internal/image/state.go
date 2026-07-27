// Package image owns reading and reporting the rover's image identity:
// version, dev/prod variant, active partition (A or B), and bootcount state.
package image

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/waypointos/waypoint/agent/internal/partition"
)

type State struct {
	Version   string
	Variant   string
	Partition string
	BuiltAt   string
	Bootcount int
}

type Loader struct {
	ImageTOMLPath string
	BootcountPath string
	CmdlinePath   string
}

func DefaultLoader() *Loader {
	return &Loader{
		ImageTOMLPath: "/etc/waypoint/image.toml",
		BootcountPath: "/data/waypoint/bootcount",
		CmdlinePath:   "/proc/cmdline",
	}
}

func (l *Loader) Load() (*State, error) {
	type tomlShape struct {
		Image struct {
			Version   string `toml:"version"`
			Variant   string `toml:"variant"`
			Partition string `toml:"partition"`
			BuiltAt   string `toml:"built_at"`
		} `toml:"image"`
	}
	// Active partition comes from the kernel cmdline at runtime, not from
	// image.toml: the same rootfs ships to both slots, so the baked
	// image.partition string would always read "A". The toml value is only a
	// fallback for when the cmdline can't be read or names no known slot.
	runtimePart, partErr := partition.Active(l.CmdlinePath)

	if _, err := os.Stat(l.ImageTOMLPath); errors.Is(err, os.ErrNotExist) {
		part := "A"
		if partErr == nil {
			part = runtimePart
		}
		return &State{Version: "0.0.0-dev", Variant: "dev", Partition: part}, nil
	}
	var t tomlShape
	if _, err := toml.DecodeFile(l.ImageTOMLPath, &t); err != nil {
		return nil, err
	}
	part := t.Image.Partition
	if partErr == nil {
		part = runtimePart
	}
	s := &State{
		Version:   t.Image.Version,
		Variant:   t.Image.Variant,
		Partition: part,
		BuiltAt:   t.Image.BuiltAt,
	}
	if b, err := os.ReadFile(l.BootcountPath); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			s.Bootcount = n
		}
	}
	return s, nil
}

// SignalHealthy writes the healthy-marker file. Pass "" to use the default
// /run/waypoint/healthy path.
func SignalHealthy(path string) error {
	if path == "" {
		path = "/run/waypoint/healthy"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
