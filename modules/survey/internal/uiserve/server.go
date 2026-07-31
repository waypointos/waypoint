// Package uiserve feeds the module's dashboard panel: it publishes the
// mission doc on the module's "mission" leaf and answers JSON requests on
// "ui.req". The panel bridge is pub/sub only, so replies are plain
// publishes on "ui.resp" correlated by request_id.
package uiserve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

// Bus is the slice of wpmodule.M the server needs; narrowed for tests.
type Bus interface {
	Publish(subject string, data []byte) error
	Subject(leaf string) string
}

// Baseline is the config-sourced mission set the override reverts to.
type Baseline struct {
	Waypoints []nav.Vec
	TagIDs    []int
	Start     nav.Pose
}

type Options struct {
	LogDir       string
	SnapDir      string
	OverridePath string
	Baseline     Baseline
}

type Server struct {
	bus   Bus
	eng   *mission.Engine
	trail *Trail
	opt   Options

	mu       sync.Mutex
	override bool
}

func New(bus Bus, eng *mission.Engine, opt Options) *Server {
	return &Server{bus: bus, eng: eng, trail: &Trail{}, opt: opt}
}

// OnPose is the engine pose listener feeding the trail.
func (s *Server) OnPose(p nav.Pose) { s.trail.Append(p) }

// SnapPath allocates the snapshot path for a sense detection, creating the
// snaps directory on first use.
func (s *Server) SnapPath(wp, id int) (string, error) {
	if err := os.MkdirAll(s.opt.SnapDir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102-150405")
	return filepath.Join(s.opt.SnapDir, fmt.Sprintf("wp%02d-id%d-%s.jpg", wp, id, ts)), nil
}

// LoadOverride applies a persisted waypoint override at startup, while the
// engine is still IDLE. A missing file is the normal config-driven case.
func (s *Server) LoadOverride() error {
	b, err := os.ReadFile(s.opt.OverridePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var ov overridePayload
	if err := json.Unmarshal(b, &ov); err != nil {
		return fmt.Errorf("override file: %w", err)
	}
	wps, tags, start, err := s.resolveOverride(ov)
	if err != nil {
		return fmt.Errorf("override file: %w", err)
	}
	if err := s.eng.Reload(wps, tags, start, time.Now()); err != nil {
		return err
	}
	s.setOverride(true)
	return nil
}

// Run publishes the mission doc at 2 Hz plus immediately on state change,
// and resets the trail when a new mission arms.
func (s *Server) Run(ctx context.Context) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	var lastState string
	var lastPub, lastEpoch time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			v := s.eng.View()
			if !v.StartedAt.Equal(lastEpoch) {
				lastEpoch = v.StartedAt
				if !v.StartedAt.IsZero() {
					s.trail.Reset()
				}
			}
			if v.State == lastState && now.Sub(lastPub) < 500*time.Millisecond {
				continue
			}
			lastState, lastPub = v.State, now
			if err := s.publishDoc(v); err != nil {
				slog.Warn("survey: mission doc publish", "err", err)
			}
		}
	}
}

// Mission doc wire shape. detected_id and tag_id use -1 for "none"/"any";
// last_detection is null until a dwell confirms an id; epoch is the mission
// start unix time (0 before the first arm), the panel's trail-reset signal.
type docWaypoint struct {
	I          int     `json:"i"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	TagID      int     `json:"tag_id"`
	Status     string  `json:"status"`
	DetectedID int     `json:"detected_id"`
}

type docPose struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Theta float64 `json:"theta"`
}

type docDetection struct {
	WP int     `json:"wp"`
	ID int     `json:"id"`
	T  float64 `json:"t"`
}

type missionDoc struct {
	State         string        `json:"state"`
	Mode          string        `json:"mode"`
	Leg           int           `json:"leg"`
	Waypoints     []docWaypoint `json:"waypoints"`
	Planned       [][2]float64  `json:"planned"`
	Pose          docPose       `json:"pose"`
	ActiveSource  string        `json:"active_source"`
	LastDetection *docDetection `json:"last_detection"`
	Epoch         float64       `json:"epoch"`
}

func (s *Server) publishDoc(v mission.View) error {
	doc := missionDoc{
		State:        v.State,
		Mode:         v.Mode,
		Leg:          v.Leg,
		Waypoints:    make([]docWaypoint, len(v.Waypoints)),
		Planned:      v.Planned,
		Pose:         docPose{X: v.Pose.X, Y: v.Pose.Y, Theta: v.Pose.Theta},
		ActiveSource: s.activeSource(),
	}
	for i, wp := range v.Waypoints {
		doc.Waypoints[i] = docWaypoint{
			I: wp.I, X: wp.X, Y: wp.Y, TagID: wp.TagID,
			Status: wp.Status, DetectedID: wp.DetectedID,
		}
	}
	if v.LastDet != nil {
		doc.LastDetection = &docDetection{
			WP: v.LastDet.WP, ID: v.LastDet.ID,
			T: float64(v.LastDet.T.UnixNano()) / 1e9,
		}
	}
	if !v.StartedAt.IsZero() {
		doc.Epoch = float64(v.StartedAt.UnixNano()) / 1e9
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return s.bus.Publish(s.bus.Subject("mission"), b)
}

func (s *Server) activeSource() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.override {
		return "override"
	}
	return "config"
}

func (s *Server) setOverride(on bool) {
	s.mu.Lock()
	s.override = on
	s.mu.Unlock()
}

// request is the ui.req wire shape; unused fields stay zero per op.
type request struct {
	RequestID string `json:"request_id"`
	Op        string `json:"op"`
	Name      string `json:"name"`
	overridePayload
}

type overridePayload struct {
	Waypoints [][]float64 `json:"waypoints"`
	TagIDs    []int       `json:"tag_ids"`
	StartPose []float64   `json:"start_pose"`
}

// HandleReq processes one ui.req message and publishes the reply on
// ui.resp. Errors become {request_id, error}.
func (s *Server) HandleReq(data []byte) {
	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		s.respond(map[string]any{"request_id": "", "error": "bad request: " + err.Error()})
		return
	}
	resp, err := s.dispatch(&req)
	if err != nil {
		s.respond(map[string]any{"request_id": req.RequestID, "error": err.Error()})
		return
	}
	resp["request_id"] = req.RequestID
	s.respond(resp)
}

func (s *Server) respond(payload map[string]any) {
	b, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("survey: ui.resp marshal", "err", err)
		return
	}
	if err := s.bus.Publish(s.bus.Subject("ui.resp"), b); err != nil {
		slog.Warn("survey: ui.resp publish", "err", err)
	}
}

func (s *Server) dispatch(req *request) (map[string]any, error) {
	switch req.Op {
	case "trail.get":
		return map[string]any{"points": s.trail.Points()}, nil
	case "logs.list":
		return s.listFiles(s.opt.LogDir, false)
	case "logs.get":
		return s.getFile(s.opt.LogDir, req.Name)
	case "snaps.list":
		return s.listFiles(s.opt.SnapDir, true)
	case "snaps.get":
		return s.getFile(s.opt.SnapDir, req.Name)
	case "waypoints.get":
		return s.waypointsGet(), nil
	case "waypoints.set":
		return s.waypointsSet(req.overridePayload)
	case "waypoints.clear":
		return s.waypointsClear()
	}
	return nil, fmt.Errorf("unknown op %q", req.Op)
}

type fileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	WP   *int   `json:"wp,omitempty"`
	ID   *int   `json:"id,omitempty"`
}

// snapNameRe extracts waypoint and tag id from wp<NN>-id<ID>-<ts>.jpg.
var snapNameRe = regexp.MustCompile(`^wp(\d+)-id(\d+)-`)

func (s *Server) listFiles(dir string, snaps bool) (map[string]any, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{"files": []fileEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fe := fileEntry{Name: e.Name(), Size: info.Size()}
		if snaps {
			if m := snapNameRe.FindStringSubmatch(e.Name()); m != nil {
				wp, _ := strconv.Atoi(m[1])
				id, _ := strconv.Atoi(m[2])
				fe.WP, fe.ID = &wp, &id
			}
		}
		files = append(files, fe)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return map[string]any{"files": files}, nil
}

func (s *Server) getFile(dir, name string) (map[string]any, error) {
	path, err := safePath(dir, name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": name, "b64": base64.StdEncoding.EncodeToString(b)}, nil
}

// safePath confines name to dir: bare filenames only, no separators or
// parent references.
func safePath(dir, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid file name %q", name)
	}
	return filepath.Join(dir, name), nil
}

func (s *Server) waypointsGet() map[string]any {
	v := s.eng.View()
	wps := make([][]float64, len(v.Waypoints))
	tags := make([]int, len(v.Waypoints))
	for i, wp := range v.Waypoints {
		wps[i] = []float64{wp.X, wp.Y}
		tags[i] = wp.TagID
	}
	return map[string]any{
		"source":     s.activeSource(),
		"waypoints":  wps,
		"tag_ids":    tags,
		"start_pose": []float64{v.Start.X, v.Start.Y, v.Start.Theta * 180 / math.Pi},
	}
}

func (s *Server) waypointsSet(ov overridePayload) (map[string]any, error) {
	wps, tags, start, err := s.resolveOverride(ov)
	if err != nil {
		return nil, err
	}
	if len(wps) == 0 {
		return nil, errors.New("waypoints.set: no waypoints given")
	}
	if err := s.eng.Reload(wps, tags, start, time.Now()); err != nil {
		return nil, err
	}
	b, err := json.Marshal(ov)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.opt.OverridePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.opt.OverridePath, b, 0o644); err != nil {
		return nil, fmt.Errorf("override applied but not persisted: %w", err)
	}
	s.setOverride(true)
	return map[string]any{"ok": true, "source": "override", "count": len(wps)}, nil
}

func (s *Server) waypointsClear() (map[string]any, error) {
	base := s.opt.Baseline
	if err := s.eng.Reload(base.Waypoints, base.TagIDs, base.Start, time.Now()); err != nil {
		return nil, err
	}
	if err := os.Remove(s.opt.OverridePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	s.setOverride(false)
	return map[string]any{"ok": true, "source": "config"}, nil
}

// resolveOverride validates the payload and fills defaults: empty tag_ids
// means accept-any, an absent start_pose keeps the config baseline.
func (s *Server) resolveOverride(ov overridePayload) ([]nav.Vec, []int, nav.Pose, error) {
	if len(ov.Waypoints) > 10 {
		return nil, nil, nav.Pose{}, fmt.Errorf("%d waypoints given, at most 10 supported", len(ov.Waypoints))
	}
	wps := make([]nav.Vec, len(ov.Waypoints))
	for i, p := range ov.Waypoints {
		if len(p) != 2 {
			return nil, nil, nav.Pose{}, fmt.Errorf("waypoint %d: want [x,y], got %d values", i, len(p))
		}
		wps[i] = nav.Vec{X: p[0], Y: p[1]}
	}
	if len(ov.TagIDs) != 0 && len(ov.TagIDs) != len(wps) {
		return nil, nil, nav.Pose{}, fmt.Errorf("%d tag ids for %d waypoints", len(ov.TagIDs), len(wps))
	}
	start := s.opt.Baseline.Start
	if len(ov.StartPose) != 0 {
		if len(ov.StartPose) != 3 {
			return nil, nil, nav.Pose{}, fmt.Errorf("start_pose: want [x,y,theta_deg], got %d values", len(ov.StartPose))
		}
		start = nav.Pose{X: ov.StartPose[0], Y: ov.StartPose[1], Theta: ov.StartPose[2] * math.Pi / 180}
	}
	return wps, ov.TagIDs, start, nil
}
