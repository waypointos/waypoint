package modules

import (
	"context"
	"sync"
	"time"
)

// noopSupervisor tracks module lifecycle state in-memory without spawning
// a process. Available on every platform so cross-platform tests (e.g.
// manager_test.go) can use it for orchestration coverage. NewDefaultSupervisor
// (build-tag-gated) returns this on non-Linux; systemd takes over on Linux.
type noopSupervisor struct {
	mu     sync.Mutex
	states map[string]Status
}

func NewNoopSupervisor() *noopSupervisor {
	return &noopSupervisor{states: map[string]Status{}}
}

func (n *noopSupervisor) Start(_ context.Context, id string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.states[id] = Status{State: StateRunning, LastRestart: time.Now()}
	return nil
}

func (n *noopSupervisor) Stop(_ context.Context, id string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.states[id] = Status{State: StateStopped}
	return nil
}

func (n *noopSupervisor) Status(id string) Status {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.states[id]
}

func (n *noopSupervisor) IsActive(_ context.Context, id string) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.states[id].State == StateRunning, nil
}
