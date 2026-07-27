package alerts

import (
	"context"
	"time"
)

// LeafState is the slice of nats-server we depend on for leaf-node liveness.
// The embedded *natsserver.Server.NumLeafNodes() satisfies this interface;
// tests substitute a fake so transitions can be driven deterministically
// without standing up a real leaf-node remote.
type LeafState interface {
	NumLeafNodes() int
}

// LeafWatcher polls a LeafState on a fixed interval and fires onDown/onUp
// callbacks on transitions between "no leaf nodes" and "≥1 leaf node". The
// agent uses it to raise/auto-resolve the proxy_disconnected alert when the
// leaf link to the Waypoint proxy hub flaps.
type LeafWatcher struct {
	state    LeafState
	interval time.Duration
	onDown   func()
	onUp     func()
}

// NewLeafWatcher returns a watcher that calls onDown on transitions from
// connected→disconnected and onUp on transitions from disconnected→connected.
// Either callback may be nil (the transition is then a silent no-op).
func NewLeafWatcher(state LeafState, interval time.Duration, onDown, onUp func()) *LeafWatcher {
	return &LeafWatcher{state: state, interval: interval, onDown: onDown, onUp: onUp}
}

// Run polls until ctx is canceled. The first observation establishes the
// baseline — no callback fires for it, so a fresh boot into a healthy leaf
// link does NOT spuriously emit an "up" event, and a boot with the proxy
// already down does NOT immediately raise (the watchdog would race with
// the leaf-node remote which may still be reconnecting).
//
// Subsequent transitions fire the matching callback exactly once per edge.
// Callbacks run inline on the polling goroutine; keep them cheap or
// hand off to another goroutine.
func (w *LeafWatcher) Run(ctx context.Context) {
	var seen, last bool // seen=baseline established; last=last observed connected?
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			connected := w.state.NumLeafNodes() > 0
			if !seen {
				last = connected
				seen = true
				continue
			}
			if connected == last {
				continue
			}
			if connected {
				if w.onUp != nil {
					w.onUp()
				}
			} else {
				if w.onDown != nil {
					w.onDown()
				}
			}
			last = connected
		}
	}
}
