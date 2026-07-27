package modules

import (
	"context"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

const defaultSnapshotInterval = time.Second

type runtimeModule struct {
	label, version string
	ui             UIBinding
	origin         waypointv1.ModuleOrigin
}

// SnapshotPublisher publishes waypoint.<rover-id>.infra.modules at a steady
// cadence and on health state changes.
type SnapshotPublisher struct {
	nc       *natsgo.Conn
	subject  string
	specs    []ModuleSpec
	interval time.Duration

	mu       sync.Mutex
	healthy  map[string]bool
	lastSeen map[string]time.Time
	runtime  map[string]runtimeModule
	change   chan struct{}
}

func NewSnapshotPublisher(nc *natsgo.Conn, roverID string, specs []ModuleSpec) *SnapshotPublisher {
	return &SnapshotPublisher{
		nc:       nc,
		subject:  "waypoint." + roverID + ".infra.modules",
		specs:    specs,
		interval: defaultSnapshotInterval,
		healthy:  map[string]bool{},
		lastSeen: map[string]time.Time{},
		runtime:  map[string]runtimeModule{},
		change:   make(chan struct{}, 1),
	}
}

func (p *SnapshotPublisher) SetInterval(d time.Duration) { p.interval = d }

func (p *SnapshotPublisher) signalChange() {
	select {
	case p.change <- struct{}{}:
	default:
	}
}

func (p *SnapshotPublisher) SetHealth(moduleID string, healthy bool) {
	p.mu.Lock()
	prev := p.healthy[moduleID]
	p.healthy[moduleID] = healthy
	if healthy {
		p.lastSeen[moduleID] = time.Now()
	}
	changed := prev != healthy
	p.mu.Unlock()
	if changed {
		p.signalChange()
	}
}

func (p *SnapshotPublisher) SetRuntimeModule(id, label, version string, ui UIBinding, origin waypointv1.ModuleOrigin) {
	p.mu.Lock()
	p.runtime[id] = runtimeModule{label: label, version: version, ui: ui, origin: origin}
	p.mu.Unlock()
	p.signalChange()
}

func (p *SnapshotPublisher) RemoveRuntimeModule(id string) {
	p.mu.Lock()
	delete(p.runtime, id)
	delete(p.healthy, id)
	delete(p.lastSeen, id)
	p.mu.Unlock()
	p.signalChange()
}

func (p *SnapshotPublisher) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.publish() // immediate snapshot on startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.publish()
		case <-p.change:
			p.publish()
		}
	}
}

func uiToProto(ui UIBinding) *waypointv1.ModuleUI {
	// A teleop-only module declares [ui.teleop] with no tab kind; it still needs
	// a ModuleUI so the window descriptor reaches the dashboard host.
	if ui.Kind == UIKindNone && ui.Teleop == nil {
		return nil
	}
	kind := waypointv1.ModuleUI_KIND_UNSPECIFIED
	switch ui.Kind {
	case UIKindStatic:
		kind = waypointv1.ModuleUI_KIND_STATIC
	case UIKindProxy:
		kind = waypointv1.ModuleUI_KIND_PROXY
	}
	out := &waypointv1.ModuleUI{Kind: kind, TabId: ui.TabID, LanOnly: ui.LANOnly}
	if ui.Teleop != nil {
		out.Teleop = &waypointv1.TeleopWindow{
			WindowId: ui.Teleop.WindowID,
			Label:    ui.Teleop.Label,
			Entry:    ui.Teleop.Entry,
			Bindings: ui.Teleop.Bindings,
		}
	}
	return out
}

func (p *SnapshotPublisher) buildSnapshot() *waypointv1.ModuleSnapshot {
	now := time.Now()
	msg := &waypointv1.ModuleSnapshot{T: timestamppb.New(now)}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.specs {
		// A reconciler-attached (runtime) module shadows a baked spec of the same
		// id, so a module present both baked and installed yields one tab, not two.
		if _, isRuntime := p.runtime[s.Manifest.Name]; isRuntime {
			continue
		}
		info := &waypointv1.ModuleInfo{
			Id:      s.Manifest.Name,
			Label:   s.Manifest.Label,
			Version: s.Manifest.Version,
			TabId:   s.Manifest.UI.TabID,
			Healthy: p.healthy[s.Manifest.Name],
			Ui:      uiToProto(s.Manifest.UI),
		}
		if s.Manifest.Component != nil {
			info.Component = &waypointv1.ModuleComponent{
				Class:       s.Manifest.Component.Class,
				StateRateHz: s.Manifest.Component.StateRateHz,
			}
		}
		if ls, ok := p.lastSeen[s.Manifest.Name]; ok {
			info.LastSeen = timestamppb.New(ls)
		}
		msg.Modules = append(msg.Modules, info)
	}
	for id, rm := range p.runtime {
		info := &waypointv1.ModuleInfo{
			Id: id, Label: rm.label, Version: rm.version,
			TabId: rm.ui.TabID, Healthy: p.healthy[id], Ui: uiToProto(rm.ui),
			Origin: rm.origin,
		}
		if ls, ok := p.lastSeen[id]; ok {
			info.LastSeen = timestamppb.New(ls)
		}
		msg.Modules = append(msg.Modules, info)
	}
	return msg
}

func (p *SnapshotPublisher) publish() {
	body, err := proto.Marshal(p.buildSnapshot())
	if err != nil {
		return
	}
	_ = p.nc.Publish(p.subject, body)
}
