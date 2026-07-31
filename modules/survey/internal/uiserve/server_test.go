package uiserve

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

type fakeBus struct {
	mu   sync.Mutex
	msgs []struct {
		Subject string
		Data    []byte
	}
}

func (b *fakeBus) Publish(subject string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msgs = append(b.msgs, struct {
		Subject string
		Data    []byte
	}{subject, data})
	return nil
}

func (b *fakeBus) Subject(leaf string) string {
	return "waypoint.test.module.survey." + leaf
}

func (b *fakeBus) last(t *testing.T) (string, map[string]any) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	require.NotEmpty(t, b.msgs)
	m := b.msgs[len(b.msgs)-1]
	var payload map[string]any
	require.NoError(t, json.Unmarshal(m.Data, &payload))
	return m.Subject, payload
}

func newTestServer(t *testing.T, wps []nav.Vec, tags []int) (*Server, *fakeBus, *mission.Engine) {
	t.Helper()
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	eng := mission.New(mission.Params{
		Waypoints: wps, TagIDs: tags,
		CruiseSpeed: 0.35, TurnRate: 0.8, ArriveRadius: 0.45, TagStop: 0.55,
		SenseDwell: time.Second, ScanMaxRad: 2, ReturnHome: true,
	})
	bus := &fakeBus{}
	s := New(bus, eng, Options{
		LogDir:       logDir,
		SnapDir:      filepath.Join(dir, "snaps"),
		OverridePath: filepath.Join(dir, "waypoints_override.json"),
		Baseline:     Baseline{Waypoints: wps, TagIDs: tags},
	})
	return s, bus, eng
}

func req(t *testing.T, s *Server, payload map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	s.HandleReq(b)
	subj, resp := s.bus.(*fakeBus).last(t)
	assert.Equal(t, "waypoint.test.module.survey.ui.resp", subj)
	assert.Equal(t, payload["request_id"], resp["request_id"])
	return resp
}

func TestHandleReqUnknownOp(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	resp := req(t, s, map[string]any{"request_id": "r1", "op": "nope"})
	assert.Contains(t, resp["error"], "unknown op")
}

func TestGetFileRejectsTraversal(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	for _, name := range []string{"../secret", "a/b.csv", "/etc/passwd", "..", ""} {
		resp := req(t, s, map[string]any{"request_id": "r", "op": "logs.get", "name": name})
		assert.Contains(t, resp, "error", "name %q must be rejected", name)
	}
}

func TestLogsListAndGetRoundtrip(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	content := []byte("t,x,y\n1,2,3\n")
	require.NoError(t, os.WriteFile(filepath.Join(s.opt.LogDir, "mission-1.csv"), content, 0o644))

	resp := req(t, s, map[string]any{"request_id": "l1", "op": "logs.list"})
	files := resp["files"].([]any)
	require.Len(t, files, 1)
	f := files[0].(map[string]any)
	assert.Equal(t, "mission-1.csv", f["name"])
	assert.EqualValues(t, len(content), f["size"])

	resp = req(t, s, map[string]any{"request_id": "l2", "op": "logs.get", "name": "mission-1.csv"})
	got, err := base64.StdEncoding.DecodeString(resp["b64"].(string))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestSnapsListEmptyWhenDirMissing(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	resp := req(t, s, map[string]any{"request_id": "s1", "op": "snaps.list"})
	assert.Empty(t, resp["files"])
}

func TestSnapsListParsesNames(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	require.NoError(t, os.MkdirAll(s.opt.SnapDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(s.opt.SnapDir, "wp02-id40-20260731-101112.jpg"), []byte("jpg"), 0o644))
	resp := req(t, s, map[string]any{"request_id": "s2", "op": "snaps.list"})
	files := resp["files"].([]any)
	require.Len(t, files, 1)
	f := files[0].(map[string]any)
	assert.EqualValues(t, 2, f["wp"])
	assert.EqualValues(t, 40, f["id"])
}

func TestWaypointsOverrideLifecycle(t *testing.T) {
	base := []nav.Vec{{X: 1, Y: 0}}
	s, _, eng := newTestServer(t, base, []int{7})

	resp := req(t, s, map[string]any{
		"request_id": "w1", "op": "waypoints.set",
		"waypoints":  [][]float64{{2, 0}, {2, 2}},
		"tag_ids":    []int{11, 40},
		"start_pose": []float64{0, 0, 90},
	})
	assert.Equal(t, true, resp["ok"])
	assert.Equal(t, "override", resp["source"])
	assert.EqualValues(t, 2, resp["count"])
	assert.FileExists(t, s.opt.OverridePath)

	v := eng.View()
	require.Len(t, v.Waypoints, 2)
	assert.Equal(t, 2.0, v.Waypoints[0].X)
	assert.Equal(t, 11, v.Waypoints[0].TagID)
	assert.InDelta(t, 1.5708, v.Start.Theta, 1e-3)

	resp = req(t, s, map[string]any{"request_id": "w2", "op": "waypoints.get"})
	assert.Equal(t, "override", resp["source"])
	assert.Len(t, resp["waypoints"], 2)

	resp = req(t, s, map[string]any{"request_id": "w3", "op": "waypoints.clear"})
	assert.Equal(t, "config", resp["source"])
	assert.NoFileExists(t, s.opt.OverridePath)
	v = eng.View()
	require.Len(t, v.Waypoints, 1)
	assert.Equal(t, 1.0, v.Waypoints[0].X)
	assert.Equal(t, 7, v.Waypoints[0].TagID)
}

func TestWaypointsSetRejectedMidMission(t *testing.T) {
	s, _, eng := newTestServer(t, []nav.Vec{{X: 3, Y: 0}}, nil)
	eng.OnMode(mission.ModeAutonomous, time.Now()) // arms: IDLE -> TRANSIT
	require.Equal(t, "TRANSIT", eng.View().State)

	resp := req(t, s, map[string]any{
		"request_id": "w4", "op": "waypoints.set",
		"waypoints": [][]float64{{9, 9}},
	})
	assert.Contains(t, resp["error"], "TRANSIT")
	assert.NoFileExists(t, s.opt.OverridePath)
	assert.Equal(t, 3.0, eng.View().Waypoints[0].X)
}

func TestWaypointsSetValidation(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	resp := req(t, s, map[string]any{"request_id": "v1", "op": "waypoints.set"})
	assert.Contains(t, resp["error"], "no waypoints")
	resp = req(t, s, map[string]any{
		"request_id": "v2", "op": "waypoints.set",
		"waypoints": [][]float64{{1, 2}}, "tag_ids": []int{1, 2},
	})
	assert.Contains(t, resp["error"], "tag ids")
}

func TestMissionDocContent(t *testing.T) {
	s, bus, eng := newTestServer(t, []nav.Vec{{X: 4, Y: 0}, {X: 4, Y: 4}}, []int{17, 42})
	s.trail.Append(nav.Pose{X: 0.5})
	require.NoError(t, s.publishDoc(eng.View()))

	subj, doc := bus.last(t)
	assert.Equal(t, "waypoint.test.module.survey.mission", subj)
	assert.Equal(t, "IDLE", doc["state"])
	assert.Equal(t, "UNKNOWN", doc["mode"])
	assert.EqualValues(t, 0, doc["leg"])
	assert.Equal(t, "config", doc["active_source"])
	assert.Nil(t, doc["last_detection"])
	assert.EqualValues(t, 0, doc["epoch"])

	wps := doc["waypoints"].([]any)
	require.Len(t, wps, 2)
	wp0 := wps[0].(map[string]any)
	assert.EqualValues(t, 0, wp0["i"])
	assert.EqualValues(t, 4, wp0["x"])
	assert.EqualValues(t, 17, wp0["tag_id"])
	assert.Equal(t, "pending", wp0["status"])
	assert.EqualValues(t, -1, wp0["detected_id"])

	// start, wp0, wp1, wp0 again, start: outbound plus reverse path home.
	planned := doc["planned"].([]any)
	assert.Len(t, planned, 5)

	pose := doc["pose"].(map[string]any)
	assert.EqualValues(t, 0, pose["x"])

	// Arming stamps the epoch and flips statuses live.
	eng.OnMode(mission.ModeAutonomous, time.Unix(1_700_000_000, 0))
	require.NoError(t, s.publishDoc(eng.View()))
	_, doc = bus.last(t)
	assert.Equal(t, "TRANSIT", doc["state"])
	assert.Equal(t, "AUTONOMOUS", doc["mode"])
	assert.EqualValues(t, 1_700_000_000, doc["epoch"])
}

func TestTrailGetOp(t *testing.T) {
	s, _, _ := newTestServer(t, nil, nil)
	s.trail.Append(nav.Pose{X: 1, Y: 2})
	resp := req(t, s, map[string]any{"request_id": "t1", "op": "trail.get"})
	pts := resp["points"].([]any)
	require.Len(t, pts, 1)
	pt := pts[0].([]any)
	assert.EqualValues(t, 1, pt[0])
	assert.EqualValues(t, 2, pt[1])
}

func TestLoadOverrideAppliesAtStartup(t *testing.T) {
	s, _, eng := newTestServer(t, []nav.Vec{{X: 1, Y: 1}}, nil)
	ov := `{"waypoints":[[5,5],[6,6]],"tag_ids":[1,2],"start_pose":[0,0,0]}`
	require.NoError(t, os.WriteFile(s.opt.OverridePath, []byte(ov), 0o644))
	require.NoError(t, s.LoadOverride())
	assert.Equal(t, "override", s.activeSource())
	assert.Len(t, eng.View().Waypoints, 2)
}
