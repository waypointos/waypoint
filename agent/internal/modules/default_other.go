//go:build !linux

package modules

import "context"

// NewDefaultSupervisor returns the no-op supervisor on non-Linux hosts.
// On Linux, the systemd backend in systemd.go takes its place.
func NewDefaultSupervisor() Supervisor { return NewNoopSupervisor() }

// reloadSystemdUnits is a no-op off Linux; the systemd version in systemd.go
// shells out to `systemctl daemon-reload`.
func reloadSystemdUnits(_ context.Context) error { return nil }
