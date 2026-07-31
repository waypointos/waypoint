package modules

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeRunner struct {
	called [][]string
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, c *exec.Cmd) error {
	f.called = append(f.called, c.Args)
	return f.err
}

func TestExecPortable_Attach(t *testing.T) {
	f := &fakeRunner{}
	p := Portable{runner: f}
	err := p.Attach(context.Background(), "/data/waypoint/modules/x/x.raw")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"portablectl", "attach", "--runtime", "--profile=trusted", "--copy=symlink", "/data/waypoint/modules/x/x.raw", "waypoint-module"}}, f.called)
}

func TestExecPortable_Detach(t *testing.T) {
	f := &fakeRunner{}
	p := Portable{runner: f}
	err := p.Detach(context.Background(), "x")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"portablectl", "detach", "--runtime", "x", "waypoint-module"}}, f.called)
}

func TestExecPortable_Mount(t *testing.T) {
	f := &fakeRunner{}
	p := Portable{runner: f}
	err := p.Mount(context.Background(), "/data/waypoint/modules/x/0.1.0/x.raw", "/run/waypoint/module-root/x")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"systemd-dissect", "--mount", "--read-only", "/data/waypoint/modules/x/0.1.0/x.raw", "/run/waypoint/module-root/x"}}, f.called)
}

func TestExecPortable_Unmount(t *testing.T) {
	f := &fakeRunner{}
	p := Portable{runner: f}
	err := p.Unmount(context.Background(), "/run/waypoint/module-root/x")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"systemd-dissect", "--umount", "/run/waypoint/module-root/x"}}, f.called)
}
