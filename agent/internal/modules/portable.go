package modules

import (
	"context"
	"fmt"
	"os/exec"
)

// PortableImage is the subset of portablectl + systemd-dissect behaviour the
// reconciler needs. Implementations: the real binaries (Portable) or a fake.
type PortableImage interface {
	Attach(ctx context.Context, rawPath string) error
	Detach(ctx context.Context, moduleID string) error
	// Mount exposes the image's read-only rootfs at mountPath. portablectl
	// mounts the image only inside the unit's namespace, so the agent mounts it
	// itself to read the manifest and serve the module's dashboard assets.
	Mount(ctx context.Context, rawPath, mountPath string) error
	Unmount(ctx context.Context, mountPath string) error
}

type runner interface {
	Run(ctx context.Context, c *exec.Cmd) error
}

type osRunner struct{}

func (osRunner) Run(_ context.Context, c *exec.Cmd) error {
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %w", c.Args[0], err)
	}
	return nil
}

type Portable struct {
	runner runner
}

func NewPortable() *Portable {
	return &Portable{runner: osRunner{}}
}

// modulePrefix is passed to portablectl explicitly: module images declare
// PORTABLE_PREFIXES=waypoint-module and ship waypoint-module-<id>.service, but
// the image filename stem is not a valid prefix, so a bare attach is rejected.
const modulePrefix = "waypoint-module"

// Attach uses --runtime (the rover root is read-only; a persistent attach would
// write units under /etc/systemd/system.attached and fail) and the "trusted"
// portable profile, which bind-mounts host /run into the unit so the module can
// read its agent-written creds/config and avoids the default profile's heavy
// sandbox (which a minimal image's rootfs can't satisfy). Modules are
// cosign-verified, so the reduced isolation is an acceptable trade.
func (p *Portable) Attach(ctx context.Context, rawPath string) error {
	cmd := newPortableCmd("attach", "--runtime", "--profile=trusted", "--copy=symlink", rawPath, modulePrefix)
	return p.runner.Run(ctx, cmd)
}

func (p *Portable) Detach(ctx context.Context, moduleID string) error {
	cmd := newPortableCmd("detach", "--runtime", moduleID)
	return p.runner.Run(ctx, cmd)
}

func (p *Portable) Mount(ctx context.Context, rawPath, mountPath string) error {
	cmd := newDissectCmd("--mount", "--read-only", rawPath, mountPath)
	return p.runner.Run(ctx, cmd)
}

func (p *Portable) Unmount(ctx context.Context, mountPath string) error {
	cmd := newDissectCmd("--umount", mountPath)
	return p.runner.Run(ctx, cmd)
}

func newPortableCmd(args ...string) *exec.Cmd { return namedCmd("portablectl", args...) }

func newDissectCmd(args ...string) *exec.Cmd { return namedCmd("systemd-dissect", args...) }

func namedCmd(bin string, args ...string) *exec.Cmd {
	path, _ := exec.LookPath(bin)
	if path == "" {
		path = bin
	}
	c := exec.Command(path, args...)
	c.Args = append([]string{bin}, args...)
	return c
}

var _ PortableImage = (*Portable)(nil)
