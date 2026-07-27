package alerts

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeState is a thread-safe stand-in for nats-server's NumLeafNodes.
// Tests flip n between 0 and >0 to drive the watcher across transitions.
type fakeState struct {
	mu sync.Mutex
	n  int
}

func (f *fakeState) NumLeafNodes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func (f *fakeState) set(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n = n
}

// waitFor polls cond every 5ms until it returns true or the deadline passes.
// Returns true if cond eventually became true. Keeps the tests robust against
// scheduling jitter without relying on long fixed sleeps.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestLeafWatcher_BaselineDisconnectedThenUpThenDown(t *testing.T) {
	t.Parallel()
	state := &fakeState{n: 0}
	var ups, downs atomic.Int32
	w := NewLeafWatcher(state, 10*time.Millisecond,
		func() { downs.Add(1) },
		func() { ups.Add(1) },
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Let baseline (n=0) be observed.
	time.Sleep(30 * time.Millisecond)
	if got := ups.Load() + downs.Load(); got != 0 {
		t.Fatalf("baseline fired callbacks: ups=%d downs=%d", ups.Load(), downs.Load())
	}

	// Transition to connected → exactly one onUp.
	state.set(1)
	if !waitFor(time.Second, func() bool { return ups.Load() == 1 }) {
		t.Fatalf("onUp didn't fire: ups=%d downs=%d", ups.Load(), downs.Load())
	}

	// Hold steady at connected for a few ticks → no extra callbacks.
	time.Sleep(50 * time.Millisecond)
	if ups.Load() != 1 || downs.Load() != 0 {
		t.Fatalf("steady connected fired extra callbacks: ups=%d downs=%d", ups.Load(), downs.Load())
	}

	// Transition back to disconnected → exactly one onDown.
	state.set(0)
	if !waitFor(time.Second, func() bool { return downs.Load() == 1 }) {
		t.Fatalf("onDown didn't fire: ups=%d downs=%d", ups.Load(), downs.Load())
	}
}

func TestLeafWatcher_BaselineConnectedThenDown(t *testing.T) {
	t.Parallel()
	state := &fakeState{n: 1}
	var ups, downs atomic.Int32
	w := NewLeafWatcher(state, 10*time.Millisecond,
		func() { downs.Add(1) },
		func() { ups.Add(1) },
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Baseline=connected must not fire onUp.
	time.Sleep(30 * time.Millisecond)
	if ups.Load() != 0 || downs.Load() != 0 {
		t.Fatalf("baseline-connected fired callbacks: ups=%d downs=%d", ups.Load(), downs.Load())
	}

	state.set(0)
	if !waitFor(time.Second, func() bool { return downs.Load() == 1 }) {
		t.Fatalf("onDown didn't fire on connected→disconnected: downs=%d", downs.Load())
	}
	if ups.Load() != 0 {
		t.Fatalf("onUp fired on connected→disconnected: ups=%d", ups.Load())
	}
}

// TestLeafWatcher_NilCallbacks proves transitions with nil hooks don't panic.
// Useful when only one direction needs a side-effect (e.g. raise-only).
func TestLeafWatcher_NilCallbacks(t *testing.T) {
	t.Parallel()
	state := &fakeState{n: 0}
	w := NewLeafWatcher(state, 5*time.Millisecond, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	state.set(1)
	time.Sleep(20 * time.Millisecond)
	state.set(0)
	time.Sleep(20 * time.Millisecond)
	// No panic = pass.
}

// TestLeafWatcher_ContextCancelExits ensures Run returns promptly when the
// context is canceled. Without this, the watcher would leak goroutines on
// agent shutdown.
func TestLeafWatcher_ContextCancelExits(t *testing.T) {
	t.Parallel()
	state := &fakeState{n: 0}
	w := NewLeafWatcher(state, 5*time.Millisecond, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s of context cancel")
	}
}
