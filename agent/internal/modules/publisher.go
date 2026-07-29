package modules

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

const defaultSnapshotInterval = time.Second

type runtimeModule struct {
	manifest Manifest
	origin   waypointv1.ModuleOrigin
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

// SetRuntimeModule records a reconciler-attached module. It takes the whole
// manifest so every snapshot field stays in sync as the manifest grows.
func (p *SnapshotPublisher) SetRuntimeModule(id string, m Manifest, origin waypointv1.ModuleOrigin) {
	p.mu.Lock()
	p.runtime[id] = runtimeModule{manifest: m, origin: origin}
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

// manifestToInfo projects a manifest into a snapshot entry. Single place, so a
// baked module and a runtime-attached one always advertise the same fields.
func manifestToInfo(m Manifest, healthy bool, origin waypointv1.ModuleOrigin) *waypointv1.ModuleInfo {
	info := &waypointv1.ModuleInfo{
		Id:      m.Name,
		Label:   m.Label,
		Version: m.Version,
		TabId:   m.UI.TabID,
		Healthy: healthy,
		Ui:      uiToProto(m.UI),
		Origin:  origin,
	}
	for i, c := range m.Components {
		if i == 0 {
			info.Component = &waypointv1.ModuleComponent{Class: c.Class, StateRateHz: c.StateRateHz}
		}
		info.Components = append(info.Components, &waypointv1.ModuleComponent{
			Class:       c.Class,
			StateRateHz: c.StateRateHz,
		})
	}
	for _, f := range m.ConfigFields {
		info.ConfigFields = append(info.ConfigFields, &waypointv1.ModuleConfigField{
			Key:          f.Key,
			Label:        f.Label,
			Type:         f.Type,
			DefaultValue: f.Default,
			Help:         f.Help,
			Required:     f.Required,
		})
	}
	return info
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
		info := manifestToInfo(s.Manifest, p.healthy[s.Manifest.Name], waypointv1.ModuleOrigin_MODULE_ORIGIN_UNSPECIFIED)
		if ls, ok := p.lastSeen[s.Manifest.Name]; ok {
			info.LastSeen = timestamppb.New(ls)
		}
		msg.Modules = append(msg.Modules, info)
	}
	// Sorted: map order would otherwise reshuffle the dashboard's module tabs
	// on every snapshot.
	for _, id := range slices.Sorted(maps.Keys(p.runtime)) {
		rm := p.runtime[id]
		info := manifestToInfo(rm.manifest, p.healthy[id], rm.origin)
		info.Id = id // the attached id wins: it is what every subject is keyed by
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
