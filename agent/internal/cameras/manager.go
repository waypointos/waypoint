package cameras

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/cameras/pipeline"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// cameraListInterval is how often the manager re-publishes camera.list. NATS
// doesn't replay messages, so a one-shot publish at boot is invisible to a
// dashboard that connects later; re-publishing at 2 s keeps fresh tab loads
// responsive (one CameraList message is ~50 bytes).
const cameraListInterval = 2 * time.Second

// cameraRescanInterval is how often auto-discovery re-scans /dev/video* for
// plug/unplug. Much slower than the publish cadence: physical hotplug is rare
// and a scan opens every video node, so 30 s is plenty and avoids poking
// streaming devices every couple seconds.
const cameraRescanInterval = 30 * time.Second

type ManagerConfig struct {
	Cameras []CameraSpec
	NC      *nats.Conn
	RoverID string
	IceCfg  func() []webrtc.ICEServer // resolved per-call
	// Discover overrides camera auto-discovery (tests inject a fake). Nil → the
	// platform default (discoverCameras: V4L2 enumeration on the Pi, none off-rover).
	Discover func() []CameraSpec
}

type CameraSpec struct {
	Name     string // "chassis-front", "gripper"
	Device   string // /dev/video0, 0 (mac avf index), or "" for synthetic
	Pipeline string // "synthetic", "mac", "pi5"; "" picks via OS detection
}

type Manager struct {
	cfg          ManagerConfig
	autoDiscover bool
	reconcileMu  sync.Mutex // serializes reconcile() (boot + ticker)
	mu           sync.Mutex
	cams         map[string]*Camera
	stop         context.CancelFunc
}

func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Discover == nil {
		cfg.Discover = discoverCameras
	}
	return &Manager{cfg: cfg, cams: map[string]*Camera{}}
}

func (m *Manager) Camera(name string) (*Camera, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cams[name]
	return c, ok
}

func (m *Manager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.cams))
	for n := range m.cams {
		out = append(out, n)
	}
	return out
}

// Tap registers fn on every currently running camera; the recording set is
// resolved at episode start, so cameras appearing later are not added.
func (m *Manager) Tap(fn func(camID string, au []byte, keyframe bool)) (remove func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	removes := make([]func(), 0, len(m.cams))
	for name, c := range m.cams {
		id := name
		removes = append(removes, c.AddTap(func(au []byte, kf bool) { fn(id, au, kf) }))
	}
	return func() {
		for _, r := range removes {
			r()
		}
	}
}

func (m *Manager) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.stop = cancel

	// No cameras declared in identity.toml → auto-discover capture devices and
	// keep re-scanning so plug/unplug is tracked live. Explicit cameras are
	// honored verbatim and never hot-swapped.
	m.autoDiscover = len(m.cfg.Cameras) == 0
	if m.autoDiscover {
		m.reconcile(ctx)
	} else {
		for _, spec := range m.cfg.Cameras {
			if err := m.startCamera(ctx, spec); err != nil {
				return err
			}
		}
	}

	if err := m.publishCameraList(); err != nil {
		return err
	}
	go m.reconcileLoop(ctx)

	subj := fmt.Sprintf("waypoint.%s.rpc.camera_offer", m.cfg.RoverID)
	if _, err := m.cfg.NC.Subscribe(subj, m.handleOfferRPC); err != nil {
		return fmt.Errorf("subscribe %s: %w", subj, err)
	}
	return nil
}

// startCamera builds the pipeline for one spec and registers the running
// camera under its name. The pipeline self-restarts on failure (see
// Camera.supervise), so this returns only on construction errors.
func (m *Manager) startCamera(ctx context.Context, spec CameraSpec) error {
	p, err := pipeline.Pick(pipeline.Spec{Name: spec.Pipeline, Device: spec.Device}, pipeline.DefaultConfig())
	if err != nil {
		return fmt.Errorf("camera %s: %w", spec.Name, err)
	}
	cam, err := New(spec.Name, p, m.cfg.IceCfg)
	if err != nil {
		return fmt.Errorf("camera %s: %w", spec.Name, err)
	}
	if err := cam.Start(ctx); err != nil {
		return fmt.Errorf("camera %s start: %w", spec.Name, err)
	}
	m.mu.Lock()
	m.cams[spec.Name] = cam
	m.mu.Unlock()
	return nil
}

// reconcile re-scans for capture devices and brings the running set in line:
// start cameras that appeared, stop cameras whose device is gone. Names are
// stable per device (camera-<videoN>), so a survivor is never restarted when a
// sibling is added or removed. Serialized so the boot call and the ticker
// can't race.
func (m *Manager) reconcile(ctx context.Context) {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()

	want := map[string]CameraSpec{}
	for _, s := range m.cfg.Discover() {
		want[s.Name] = s
	}

	m.mu.Lock()
	have := make(map[string]struct{}, len(m.cams))
	for name := range m.cams {
		have[name] = struct{}{}
	}
	m.mu.Unlock()

	for name, spec := range want {
		if _, ok := have[name]; ok {
			continue
		}
		if err := m.startCamera(ctx, spec); err != nil {
			slog.Warn(fmt.Sprintf("cameras: start discovered %s: %v", name, err))
			continue
		}
		slog.Info(fmt.Sprintf("cameras: %s appeared (%s)", name, spec.Device))
	}

	for name := range have {
		if _, ok := want[name]; ok {
			continue
		}
		m.mu.Lock()
		cam := m.cams[name]
		delete(m.cams, name)
		m.mu.Unlock()
		if cam != nil {
			cam.Stop()
			slog.Info(fmt.Sprintf("cameras: %s disappeared", name))
		}
	}
}

func (m *Manager) reconcileLoop(ctx context.Context) {
	pub := time.NewTicker(cameraListInterval)
	defer pub.Stop()

	// Re-scan for hotplug only in auto-discovery mode, and far less often than
	// we republish — physical plug/unplug is rare and a scan opens every video
	// node. Explicit configs never hot-swap, so they only get the republish.
	var rescan <-chan time.Time
	if m.autoDiscover {
		t := time.NewTicker(cameraRescanInterval)
		defer t.Stop()
		rescan = t.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-rescan:
			m.reconcile(ctx)
			_ = m.publishCameraList() // reflect a hotplug change immediately
		case <-pub.C:
			_ = m.publishCameraList()
		}
	}
}

func (m *Manager) Stop() {
	if m.stop != nil {
		m.stop()
	}
	m.mu.Lock()
	for _, c := range m.cams {
		c.Stop()
	}
	m.cams = map[string]*Camera{}
	m.mu.Unlock()
}

func (m *Manager) publishCameraList() error {
	msg := &waypointv1.CameraList{}
	m.mu.Lock()
	for name, cam := range m.cams {
		msg.Cameras = append(msg.Cameras, &waypointv1.CameraInfo{
			Name:     name,
			Pipeline: cam.pipeline.Name(),
		})
	}
	m.mu.Unlock()
	body, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	subj := fmt.Sprintf("waypoint.%s.infra.camera.list", m.cfg.RoverID)
	return m.cfg.NC.Publish(subj, body)
}

func (m *Manager) handleOfferRPC(msg *nats.Msg) {
	var req waypointv1.CameraOfferRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		_ = m.replyOfferErr(msg, "bad request: "+err.Error())
		return
	}
	cam, ok := m.Camera(req.Camera)
	if !ok {
		_ = m.replyOfferErr(msg, "no such camera")
		return
	}
	answer, err := cam.NewSubscriber(context.Background(), req.SessionId,
		webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.OfferSdp},
	)
	if err != nil {
		_ = m.replyOfferErr(msg, "subscribe: "+err.Error())
		return
	}
	resp := &waypointv1.CameraOfferResponse{AnswerSdp: answer.SDP}
	body, _ := proto.Marshal(resp)
	_ = msg.Respond(body)
}

func (m *Manager) replyOfferErr(msg *nats.Msg, reason string) error {
	r := reason
	resp := &waypointv1.CameraOfferResponse{Error: &r}
	body, _ := proto.Marshal(resp)
	return msg.Respond(body)
}
