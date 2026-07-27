// Package descriptor parses and validates the platform descriptor, the single
// source of truth for a robot's shape (schema: protocol/platform/SCHEMA.md).
package descriptor

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Descriptor struct {
	Schema       int               `toml:"schema"`
	Platform     Platform          `toml:"platform"`
	Drivers      map[string]Driver `toml:"drivers"`
	Joints       []Joint           `toml:"joints"`
	Kinematics   *Kinematics       `toml:"kinematics"`
	Allocation   *Allocation       `toml:"allocation"`
	Sensors      []Sensor          `toml:"sensors"`
	Modes        *Modes            `toml:"modes"`
	Observations *Observations     `toml:"observations"`
	Actions      *Actions          `toml:"actions"`
}

type Platform struct {
	ID           string `toml:"id"`
	Name         string `toml:"name"`
	VehicleClass string `toml:"vehicle_class"`
}

type Driver struct {
	Kind string `toml:"kind"`
	Port string `toml:"port"`
	Baud int    `toml:"baud"`
}

type Joint struct {
	Name              string   `toml:"name"`
	Driver            string   `toml:"driver"`
	BusID             uint32   `toml:"bus_id"`
	Type              string   `toml:"type"`
	Ownership         string   `toml:"ownership"`
	Invert            bool     `toml:"invert"`
	CommandInterfaces []string `toml:"command_interfaces"`
	StateInterfaces   []string `toml:"state_interfaces"`
	Limits            *Limits  `toml:"limits"`
}

type Limits struct {
	VelocityRadps  *float64 `toml:"velocity_radps"`
	PositionMinRad *float64 `toml:"position_min_rad"`
	PositionMaxRad *float64 `toml:"position_max_rad"`
}

type Kinematics struct {
	Model        string            `toml:"model"`
	WheelRadiusM float64           `toml:"wheel_radius_m"`
	TrackWidthM  float64           `toml:"track_width_m"`
	Wheels       map[string]string `toml:"wheels"`
}

// Allocation is required only for vehicle classes with an explicit
// effectiveness matrix (none in schema v1; diff_drive derives from kinematics).
type Allocation struct{}

type Sensor struct {
	Name   string  `toml:"name"`
	Kind   string  `toml:"kind"`
	Frame  string  `toml:"frame"`
	RateHz float64 `toml:"rate_hz"`
}

type Modes struct {
	Available []string `toml:"available"`
}

type Observations struct {
	Streams []Stream `toml:"streams"`
}

type Stream struct {
	Subject string  `toml:"subject"`
	Message string  `toml:"message"`
	RateHz  float64 `toml:"rate_hz"`
}

type Actions struct {
	Altitudes []string `toml:"altitudes"`
}

// Parse decodes and validates a descriptor. Unknown keys are errors.
func Parse(data []byte) (*Descriptor, error) {
	var d Descriptor
	md, err := toml.Decode(string(data), &d)
	if err != nil {
		return nil, fmt.Errorf("descriptor: %w", err)
	}
	if un := md.Undecoded(); len(un) > 0 {
		return nil, fmt.Errorf("descriptor: unknown key %q", un[0].String())
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Load reads and parses a descriptor file.
func Load(path string) (*Descriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}
