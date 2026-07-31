// Binary waypoint-module-survey runs the competition survey mission:
// autonomous waypoint navigation with ArUco tag sensing, a WS2812 status
// strip, and a CSV mission log. Drive access is brokered by the agent via
// the drive-control capability; the platform only relays commands while
// the rover is in MODE_AUTONOMOUS.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sdk/wpmodule"

	"github.com/waypointos/waypoint/modules/survey/internal/config"
	"github.com/waypointos/waypoint/modules/survey/internal/led"
	"github.com/waypointos/waypoint/modules/survey/internal/mission"
	"github.com/waypointos/waypoint/modules/survey/internal/misslog"
	"github.com/waypointos/waypoint/modules/survey/internal/nav"
	"github.com/waypointos/waypoint/modules/survey/internal/selftest"
	"github.com/waypointos/waypoint/modules/survey/internal/vision"
)

const moduleID = "survey"

func main() {
	configPath := flag.String("config", "", "path to config.toml")
	credsPath := flag.String("creds", "", "path to creds.env")
	roverID := flag.String("rover", "", "rover id")
	natsURL := flag.String("nats", "", "nats URL")
	selfTest := flag.Bool("selftest", false, "run the scripted mission simulation and exit")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if *selfTest {
		if err := selftest.Run(os.Stdout); err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		return
	}

	// The unit passes flags; the SDK and dev loop use env. Flags win.
	setEnvFromFlag("WAYPOINT_MODULE_CONFIG", *configPath)
	setEnvFromFlag("WAYPOINT_MODULE_CREDS", *credsPath)
	setEnvFromFlag("WAYPOINT_ROVER_ID", *roverID)
	setEnvFromFlag("WAYPOINT_NATS_URL", *natsURL)

	err := wpmodule.Run(context.Background(), wpmodule.Options{ID: moduleID}, setup)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func setEnvFromFlag(key, val string) {
	if val != "" {
		os.Setenv(key, val)
	}
}

func setup(m *wpmodule.M) error {
	raw := map[string]any{}
	if _, err := wpmodule.LoadConfig(&raw); err != nil {
		return err
	}
	cfg, err := config.Resolve(raw)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if len(cfg.Waypoints) == 0 {
		slog.Warn("survey: no waypoints configured; module stays idle")
	}

	logger, logPath, err := misslog.New(cfg.LogDir)
	if err != nil {
		return err
	}
	slog.Info("survey: mission log", "path", logPath)

	eng := mission.New(mission.Params{
		Waypoints:    cfg.Waypoints,
		TagIDs:       cfg.TagIDs,
		Start:        cfg.Start,
		CruiseSpeed:  cfg.CruiseSpeedMps,
		TurnRate:     cfg.TurnRateRadps,
		ArriveRadius: cfg.ArriveRadiusM,
		TagStop:      cfg.TagStopM,
		SenseDwell:   time.Duration(cfg.SenseDwellS * float64(time.Second)),
		ScanMaxRad:   cfg.ScanMaxDeg * degToRad,
		ReturnHome:   cfg.ReturnHome,
	})

	strip := openStrip(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-m.Done()
		cancel()
		strip.Close()
		logger.Close()
	}()

	proc := startVision(ctx, cfg, eng)

	onTelemetry := func(msg *natsgo.Msg) {
		var tel waypointv1.DriveTelemetry
		if err := proto.Unmarshal(msg.Data, &tel); err != nil {
			return
		}
		// Optional fields: an absent value is N/A, not zero; skip the sample.
		if tel.BodyVxMps == nil && tel.YawRateRadps == nil {
			return
		}
		eng.OnOdometry(tel.GetBodyVxMps(), tel.GetYawRateRadps(), time.Now())
	}
	onMode := func(msg *natsgo.Msg) {
		var ev waypointv1.ModeEvent
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			return
		}
		eng.OnMode(mission.Mode(ev.To), time.Now())
	}

	if _, err := m.Subscribe(m.Subject("drive.telemetry"), onTelemetry); err != nil {
		return err
	}
	if _, err := m.Subscribe(m.Subject("mode"), onMode); err != nil {
		return err
	}
	// Dev harness runs the module on the open default user, and the agent's
	// per-module mirrors only exist for attached modules. Read the platform
	// subjects directly there; the minted ACL forbids this in production.
	if os.Getenv("WAYPOINT_MODULE_CREDS") == "" {
		platform := func(leaf string) string {
			return fmt.Sprintf("waypoint.%s.%s", m.RoverID(), leaf)
		}
		if _, err := m.NC().Subscribe(platform("telemetry.drive"), onTelemetry); err != nil {
			return err
		}
		if _, err := m.NC().Subscribe(platform("event.mode"), onMode); err != nil {
			return err
		}
	}

	go controlLoop(ctx, m, cfg, eng, strip, logger, proc)
	return nil
}

const degToRad = 3.14159265358979323846 / 180

// controlLoop is the 20 Hz heart: tick the engine, publish the drive
// setpoint, refresh the strip, and write CSV samples at 5 Hz.
func controlLoop(ctx context.Context, m *wpmodule.M, cfg config.Config, eng *mission.Engine,
	strip led.Driver, logger *misslog.Logger, proc *vision.Proc) {
	subj := m.Subject("drive.cmd")
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	n := 0
	camFault := false
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			out := eng.Tick(now)
			if out.Drive != nil {
				cmd := &waypointv1.DriveCommand{
					T:            timestamppb.New(now),
					BodyVxMps:    proto.Float64(out.Drive.VX),
					YawRateRadps: proto.Float64(out.Drive.YawRate),
				}
				if b, err := proto.Marshal(cmd); err == nil {
					if err := m.Publish(subj, b); err != nil {
						slog.Warn("survey: drive publish", "err", err)
					}
				}
			}
			if err := strip.Frame(renderLED(out.LED, cfg)); err != nil {
				slog.Warn("survey: led", "err", err)
			}

			snap := eng.Snapshot()
			for _, ev := range out.Events {
				slog.Info("survey: "+ev.Kind, "detail", ev.Detail)
				logRow(logger, ev.T, snap, ev.Kind+" "+ev.Detail)
			}
			if proc != nil && proc.Fault() != camFault {
				camFault = proc.Fault()
				logRow(logger, now, snap, camEvent(camFault))
			}
			if n%5 == 0 {
				logRow(logger, now, snap, "")
			}
			n++
		}
	}
}

func camEvent(fault bool) string {
	if fault {
		return "camera_fault"
	}
	return "camera_ok"
}

func logRow(logger *misslog.Logger, t time.Time, snap mission.Snap, event string) {
	err := logger.Row(t, snap.Pose.X, snap.Pose.Y, snap.Pose.Theta/degToRad,
		snap.SpeedVX, snap.State, snap.FreshID, event)
	if err != nil {
		slog.Warn("survey: mission log", "err", err)
	}
}

func renderLED(st mission.LEDState, cfg config.Config) []led.RGB {
	p := led.Pattern{ID: st.ID, ShowID: st.ShowID}
	switch st.Phase {
	case mission.LEDGreen:
		p.Base = led.Green
	case mission.LEDRed:
		p.Base = led.Red
	case mission.LEDYellow:
		p.Base = led.Yellow
	}
	return led.Render(p, cfg.LEDCount, cfg.LEDBrightness)
}

// openStrip returns the SPI driver, or the log driver in sim mode or when
// the device is unavailable; a missing strip must never stop the mission.
func openStrip(cfg config.Config) led.Driver {
	if cfg.SimMode {
		return &led.LogDriver{}
	}
	d, err := led.OpenSPI(cfg.LEDDevice)
	if err != nil {
		slog.Warn("survey: led strip unavailable, logging frames instead", "err", err)
		return &led.LogDriver{}
	}
	return d
}

// startVision launches the detector child, or the synthetic source in sim
// mode. Returns the child supervisor for fault reporting (nil in sim).
func startVision(ctx context.Context, cfg config.Config, eng *mission.Engine) *vision.Proc {
	handler := func(dets []mission.Detection, now time.Time) {
		eng.OnDetections(dets, now)
	}
	if cfg.SimMode {
		tags := make([]int, len(cfg.Waypoints))
		for i := range tags {
			tags[i] = cfg.SimTagFor(i)
		}
		sim := &vision.Sim{
			Waypoints:  cfg.Waypoints,
			Tags:       tags,
			Visibility: cfg.SimVisibilityM,
			HfovDeg:    cfg.CameraHfovDeg,
			Pose:       func() nav.Pose { return eng.Snapshot().Pose },
		}
		go sim.Run(ctx, handler)
		return nil
	}
	proc := &vision.Proc{
		Script:      findDetectScript(),
		Device:      cfg.CameraDevice,
		Width:       cfg.CameraWidth,
		Height:      cfg.CameraHeight,
		HfovDeg:     cfg.CameraHfovDeg,
		MarkerSizeM: cfg.MarkerSizeM,
	}
	go proc.Run(ctx, handler)
	return proc
}

// findDetectScript resolves detect.py: the baked image path first, then
// dev-tree locations relative to the binary and the working directory.
func findDetectScript() string {
	candidates := []string{
		"/usr/share/waypoint/modules/survey/vision/detect.py",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "vision", "detect.py"))
	}
	candidates = append(candidates,
		"modules/survey/vision/detect.py",
		"vision/detect.py",
	)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}
