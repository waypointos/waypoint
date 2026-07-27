package modules

import (
	"context"
	"time"
)

// State is a module process state from the supervisor's perspective.
type State int

const (
	StateUnknown State = iota
	StateStopped
	StateRunning
	StateFailed
)

// Status is a supervisor's view of a module process.
type Status struct {
	State       State
	PID         int
	LastRestart time.Time
	LastExitErr string
}

// Supervisor manages module process lifecycle. Implementations:
//
//   - systemdSupervisor (Linux): shells out to systemctl against a
//     concrete unit "waypoint-module-<id>.service".
//   - noopSupervisor (non-Linux + tests): tracks state in-memory; never
//     spawns a process.
//
// NewDefaultSupervisor returns the right one for the build target.
type Supervisor interface {
	Start(ctx context.Context, moduleID string) error
	Stop(ctx context.Context, moduleID string) error
	Status(moduleID string) Status
	IsActive(ctx context.Context, moduleID string) (bool, error)
}
