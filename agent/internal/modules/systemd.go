//go:build linux

package modules

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// NewDefaultSupervisor returns the systemd supervisor on Linux.
func NewDefaultSupervisor() Supervisor { return NewSystemdSupervisor() }

type systemdSupervisor struct {
	mu     sync.Mutex
	states map[string]Status
}

func NewSystemdSupervisor() *systemdSupervisor {
	return &systemdSupervisor{states: map[string]Status{}}
}

func (s *systemdSupervisor) Start(ctx context.Context, id string) error {
	// `start` (not `enable --now`). Waypoint's rootfs is a read-only
	// squashfs; `systemctl enable` would try to write the wants-symlink
	// under /etc/systemd/system/multi-user.target.wants/ and fail with
	// EROFS. Persistence isn't needed anyway — the agent re-decides
	// which module units to start on every boot based on modules.toml.
	if err := s.systemctl(ctx, "start", unitName(id)); err != nil {
		s.set(id, Status{State: StateFailed, LastExitErr: err.Error()})
		return err
	}
	s.set(id, Status{State: StateRunning, LastRestart: time.Now()})
	return nil
}

func (s *systemdSupervisor) Stop(ctx context.Context, id string) error {
	if err := s.systemctl(ctx, "stop", unitName(id)); err != nil {
		return err
	}
	s.set(id, Status{State: StateStopped})
	return nil
}

func (s *systemdSupervisor) Status(id string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[id]
}

func (s *systemdSupervisor) systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *systemdSupervisor) set(id string, st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[id] = st
}

func unitName(id string) string { return "waypoint-module-" + id + ".service" }

// reloadSystemdUnits runs `systemctl daemon-reload` so drop-ins written after
// portablectl's attach-time reload are picked up before the module starts.
func reloadSystemdUnits(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *systemdSupervisor) IsActive(ctx context.Context, id string) (bool, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", unitName(id))
	if err := cmd.Run(); err != nil {
		// Exit code 3 = inactive/failed; that's a clean "false", not an error.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
