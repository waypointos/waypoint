// Package config resolves the module's config.toml into typed mission
// settings. Values are coerced from string, integer, or float forms so a
// config written by hand or by the dashboard form both parse.
package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

type Config struct {
	Waypoints []nav.Vec
	TagIDs    []int // per waypoint; -1 = accept any
	Start     nav.Pose

	CruiseSpeedMps float64
	TurnRateRadps  float64
	ArriveRadiusM  float64
	TagStopM       float64
	SenseDwellS    float64
	ScanMaxDeg     float64
	ReturnHome     bool

	CameraDevice  string
	CameraWidth   int
	CameraHeight  int
	CameraHfovDeg float64
	MarkerSizeM   float64

	LEDDevice     string
	LEDCount      int
	LEDBrightness float64

	SimMode        bool
	SimTags        []int
	SimVisibilityM float64

	LogDir string
}

// Resolve applies defaults and parses the composite string fields. The raw
// map comes from decoding config.toml (or is empty for a bare dev run).
func Resolve(raw map[string]any) (Config, error) {
	c := Config{
		CruiseSpeedMps: 0.35,
		TurnRateRadps:  0.8,
		ArriveRadiusM:  0.45,
		TagStopM:       0.55,
		SenseDwellS:    4,
		ScanMaxDeg:     120,
		ReturnHome:     true,
		CameraDevice:   "/dev/video0",
		CameraWidth:    1280,
		CameraHeight:   720,
		CameraHfovDeg:  66,
		MarkerSizeM:    0.20,
		LEDDevice:      "/dev/spidev0.0",
		LEDCount:       8,
		LEDBrightness:  0.4,
		SimVisibilityM: 3.0,
		LogDir:         "/var/lib/waypoint-module-survey/logs",
	}

	var err error
	if s := getString(raw, "waypoints"); s != "" {
		if c.Waypoints, err = ParseWaypoints(s); err != nil {
			return c, err
		}
	}
	if s := getString(raw, "tag_ids"); s != "" {
		if c.TagIDs, err = ParseIntList(s); err != nil {
			return c, fmt.Errorf("tag_ids: %w", err)
		}
		if len(c.TagIDs) != len(c.Waypoints) {
			return c, fmt.Errorf("tag_ids: %d ids for %d waypoints", len(c.TagIDs), len(c.Waypoints))
		}
	}
	if len(c.TagIDs) == 0 {
		c.TagIDs = make([]int, len(c.Waypoints))
		for i := range c.TagIDs {
			c.TagIDs[i] = -1
		}
	}
	if s := getString(raw, "start_pose"); s != "" {
		if c.Start, err = ParseStartPose(s); err != nil {
			return c, err
		}
	}

	getFloat(raw, "cruise_speed_mps", &c.CruiseSpeedMps)
	getFloat(raw, "turn_rate_radps", &c.TurnRateRadps)
	getFloat(raw, "arrive_radius_m", &c.ArriveRadiusM)
	getFloat(raw, "tag_stop_m", &c.TagStopM)
	getFloat(raw, "sense_dwell_s", &c.SenseDwellS)
	getFloat(raw, "scan_max_deg", &c.ScanMaxDeg)
	getBool(raw, "return_home", &c.ReturnHome)

	getStringInto(raw, "camera_device", &c.CameraDevice)
	getInt(raw, "camera_width", &c.CameraWidth)
	getInt(raw, "camera_height", &c.CameraHeight)
	getFloat(raw, "camera_hfov_deg", &c.CameraHfovDeg)
	getFloat(raw, "marker_size_m", &c.MarkerSizeM)

	getStringInto(raw, "led_device", &c.LEDDevice)
	getInt(raw, "led_count", &c.LEDCount)
	getFloat(raw, "led_brightness", &c.LEDBrightness)

	getBool(raw, "sim_mode", &c.SimMode)
	getFloat(raw, "sim_visibility_m", &c.SimVisibilityM)
	if s := getString(raw, "sim_tags"); s != "" {
		if c.SimTags, err = ParseIntList(s); err != nil {
			return c, fmt.Errorf("sim_tags: %w", err)
		}
	}

	getStringInto(raw, "log_dir", &c.LogDir)

	if c.CruiseSpeedMps <= 0 || c.TurnRateRadps <= 0 {
		return c, fmt.Errorf("cruise_speed_mps and turn_rate_radps must be positive")
	}
	if c.LEDBrightness < 0 || c.LEDBrightness > 1 {
		return c, fmt.Errorf("led_brightness %v out of [0,1]", c.LEDBrightness)
	}
	return c, nil
}

// SimTagFor picks the marker id the sim vision source fakes for a waypoint:
// explicit sim_tags first, then the expected tag, then a stable fallback.
func (c Config) SimTagFor(i int) int {
	if i < len(c.SimTags) {
		return c.SimTags[i]
	}
	if i < len(c.TagIDs) && c.TagIDs[i] >= 0 {
		return c.TagIDs[i]
	}
	return i + 1
}

// ParseWaypoints parses "x1,y1;x2,y2;..." (meters, local frame).
func ParseWaypoints(s string) ([]nav.Vec, error) {
	var out []nav.Vec
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var v nav.Vec
		if err := parseFloats(part, &v.X, &v.Y); err != nil {
			return nil, fmt.Errorf("waypoints: %q: %w", part, err)
		}
		out = append(out, v)
	}
	if len(out) > 10 {
		return nil, fmt.Errorf("waypoints: %d given, at most 10 supported", len(out))
	}
	return out, nil
}

// ParseIntList parses "17,40,...".
func ParseIntList(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("%q is not an integer", part)
		}
		out = append(out, n)
	}
	return out, nil
}

// ParseStartPose parses "x,y,theta_deg".
func ParseStartPose(s string) (nav.Pose, error) {
	var x, y, deg float64
	if err := parseFloats(s, &x, &y, &deg); err != nil {
		return nav.Pose{}, fmt.Errorf("start_pose: %q: %w", s, err)
	}
	return nav.Pose{X: x, Y: y, Theta: deg * math.Pi / 180}, nil
}

func parseFloats(s string, dst ...*float64) error {
	parts := strings.Split(s, ",")
	if len(parts) != len(dst) {
		return fmt.Errorf("want %d comma-separated numbers, got %d", len(dst), len(parts))
	}
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return err
		}
		*dst[i] = f
	}
	return nil
}

func getString(raw map[string]any, key string) string {
	if v, ok := raw[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getStringInto(raw map[string]any, key string, dst *string) {
	if v, ok := raw[key]; ok {
		if s := fmt.Sprintf("%v", v); s != "" {
			*dst = s
		}
	}
}

func getFloat(raw map[string]any, key string, dst *float64) {
	v, ok := raw[key]
	if !ok {
		return
	}
	switch t := v.(type) {
	case float64:
		*dst = t
	case int64:
		*dst = float64(t)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			*dst = f
		}
	}
}

func getInt(raw map[string]any, key string, dst *int) {
	var f float64
	ok := false
	switch t := raw[key].(type) {
	case float64:
		f, ok = t, true
	case int64:
		f, ok = float64(t), true
	case string:
		if p, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			f, ok = p, true
		}
	}
	if ok {
		*dst = int(f)
	}
}

func getBool(raw map[string]any, key string, dst *bool) {
	switch t := raw[key].(type) {
	case bool:
		*dst = t
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(t)); err == nil {
			*dst = b
		}
	}
}
