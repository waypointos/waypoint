package modules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

type fakePortable struct {
	mu       sync.Mutex
	attached map[string]string // moduleID → raw path
}

func newFakePortable() *fakePortable {
	return &fakePortable{attached: map[string]string{}}
}

func (f *fakePortable) Attach(_ context.Context, rawPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirror real portablectl: the portable name is the image filename stem
	// (no version truncation), e.g. .../power-monitor.raw -> power-monitor.
	name := strings.TrimSuffix(filepath.Base(rawPath), ".raw")
	f.attached[name] = rawPath
	return nil
}

func (f *fakePortable) Detach(_ context.Context, moduleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.attached, moduleID)
	return nil
}

func (f *fakePortable) Mount(_ context.Context, _, _ string) error { return nil }
func (f *fakePortable) Unmount(_ context.Context, _ string) error  { return nil }

func (f *fakePortable) Attached() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for k, v := range f.attached {
		out[k] = v
	}
	return out
}

type fakeFetcher struct {
	bytes map[string][]byte
}

func (f *fakeFetcher) Fetch(_ context.Context, url, _, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, f.bytes[url], 0o644)
}

type fakeSupervisor struct {
	started []string
	stopped []string
	active  bool
}

func (f *fakeSupervisor) Start(_ context.Context, moduleID string) error {
	f.started = append(f.started, moduleID)
	return nil
}
func (f *fakeSupervisor) Stop(_ context.Context, moduleID string) error {
	f.stopped = append(f.stopped, moduleID)
	return nil
}
func (f *fakeSupervisor) Status(_ string) Status { return Status{} }
func (f *fakeSupervisor) IsActive(_ context.Context, _ string) (bool, error) {
	return f.active, nil
}

func TestReconcile_AttachNewModule(t *testing.T) {
	root := t.TempDir()
	port := newFakePortable()
	fet := &fakeFetcher{bytes: map[string][]byte{"https://p/x.raw": []byte("raw bytes")}}
	sup := &fakeSupervisor{}
	r := &Reconciler{
		RootDir:    root,
		Portable:   port,
		Fetcher:    fet,
		Supervisor: sup,
	}
	desired := &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{
			{Id: "power-monitor", Version: "0.1.0", RawUrl: "https://p/x.raw", RawSha256: ""},
		},
	}
	err := r.Reconcile(context.Background(), desired)
	require.NoError(t, err)
	require.Contains(t, port.Attached(), "power-monitor")
	require.Equal(t, []string{"power-monitor"}, sup.started)
}

func TestReconcile_DetachRemovedModule(t *testing.T) {
	root := t.TempDir()
	port := newFakePortable()
	port.attached["old"] = filepath.Join(root, "modules/old/0.1.0/old.raw")
	sup := &fakeSupervisor{}
	r := &Reconciler{
		RootDir:    root,
		Portable:   port,
		Fetcher:    &fakeFetcher{},
		Supervisor: sup,
		current:    map[string]string{"old": filepath.Join(root, "modules/old/0.1.0/old.raw")},
	}
	err := r.Reconcile(context.Background(), &waypointv1.DesiredModuleSet{})
	require.NoError(t, err)
	require.NotContains(t, port.Attached(), "old")
	require.Equal(t, []string{"old"}, sup.stopped)
}

func TestReconcile_VersionUpgrade(t *testing.T) {
	root := t.TempDir()
	port := newFakePortable()
	port.attached["x"] = filepath.Join(root, "modules/x/0.1.0/x.raw")
	sup := &fakeSupervisor{}
	fet := &fakeFetcher{bytes: map[string][]byte{"https://p/x-0.2.0.raw": []byte("v2 bytes")}}
	r := &Reconciler{
		RootDir:    root,
		Portable:   port,
		Fetcher:    fet,
		Supervisor: sup,
		current:    map[string]string{"x": filepath.Join(root, "modules/x/0.1.0/x.raw")},
	}
	desired := &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{
			{Id: "x", Version: "0.2.0", RawUrl: "https://p/x-0.2.0.raw"},
		},
	}
	err := r.Reconcile(context.Background(), desired)
	require.NoError(t, err)
	require.Equal(t, []string{"x"}, sup.stopped)
	require.Equal(t, []string{"x"}, sup.started)
	require.Contains(t, port.Attached()["x"], filepath.Join("0.2.0", "x.raw"))
}

func TestReconcile_FiresOnAttachAndOnDetach(t *testing.T) {
	dir := t.TempDir()
	rawDir := filepath.Join(dir, "modules", "umr", "0.1.0")
	require.NoError(t, os.MkdirAll(rawDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rawDir, "umr.raw"), []byte("x"), 0o644))

	var attached, detached []string
	r := &Reconciler{
		RootDir:    dir,
		Portable:   newFakePortable(),
		Fetcher:    &fakeFetcher{},
		Supervisor: &fakeSupervisor{},
		OnAttach:   func(id string, m *Manifest) { attached = append(attached, id) },
		OnDetach:   func(id string) { detached = append(detached, id) },
	}
	ctx := context.Background()
	require.NoError(t, r.Reconcile(ctx, &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{{Id: "umr", Version: "0.1.0"}},
	}))
	require.Equal(t, []string{"umr"}, attached)
	require.NoError(t, r.Reconcile(ctx, &waypointv1.DesiredModuleSet{}))
	require.Equal(t, []string{"umr"}, detached)
}

func TestReconcile_VersionSuffixCollision(t *testing.T) {
	// Upgrade to a version ("1.0") whose string is a substring of the previous
	// one ("0.1.0"); versions live in distinct directories, so the diff is exact.
	root := t.TempDir()
	port := newFakePortable()
	port.attached["x"] = filepath.Join(root, "modules/x/0.1.0/x.raw")
	sup := &fakeSupervisor{}
	fet := &fakeFetcher{bytes: map[string][]byte{"https://p/x-1.0.raw": []byte("v1 bytes")}}
	r := &Reconciler{
		RootDir:    root,
		Portable:   port,
		Fetcher:    fet,
		Supervisor: sup,
		current:    map[string]string{"x": filepath.Join(root, "modules/x/0.1.0/x.raw")},
	}
	desired := &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{
			{Id: "x", Version: "1.0", RawUrl: "https://p/x-1.0.raw"},
		},
	}
	err := r.Reconcile(context.Background(), desired)
	require.NoError(t, err)
	require.Equal(t, []string{"x"}, sup.stopped, "should detach old 0.1.0")
	require.Equal(t, []string{"x"}, sup.started, "should re-attach new 1.0")
	require.Contains(t, port.Attached()["x"], filepath.Join("1.0", "x.raw"))
}
