package harness

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Msg is one recorded bus message.
type Msg struct {
	Subject string // full subject
	Leaf    string // after "waypoint.<id>."
	Data    []byte
	At      time.Time // receive time (wall); capture stamps live in Data
}

// Trace records every message under waypoint.<id>.> and answers queries.
// WaitFor first scans already-recorded messages, then blocks for new ones.
type Trace struct {
	roverID string
	mu      sync.Mutex
	msgs    []Msg
	waiters []chan struct{}
}

func newTrace(roverID string) *Trace { return &Trace{roverID: roverID} }

func (t *Trace) record(subject string, data []byte) {
	leaf := strings.TrimPrefix(subject, "waypoint."+t.roverID+".")
	t.mu.Lock()
	t.msgs = append(t.msgs, Msg{Subject: subject, Leaf: leaf, Data: data, At: time.Now()})
	for _, w := range t.waiters {
		close(w)
	}
	t.waiters = nil
	t.mu.Unlock()
}

func (t *Trace) Messages(leaf string) []Msg {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Msg
	for _, m := range t.msgs {
		if m.Leaf == leaf {
			out = append(out, m)
		}
	}
	return out
}

func (t *Trace) Clear() {
	t.mu.Lock()
	t.msgs = nil
	t.mu.Unlock()
}

// WaitFor returns the first message on leaf satisfying pred, scanning the
// existing trace first and then waiting up to timeout for new messages.
func (t *Trace) WaitFor(leaf string, pred func(Msg) bool, timeout time.Duration) (Msg, error) {
	deadline := time.Now().Add(timeout)
	scanned := 0
	for {
		t.mu.Lock()
		for ; scanned < len(t.msgs); scanned++ {
			m := t.msgs[scanned]
			if m.Leaf == leaf && pred(m) {
				t.mu.Unlock()
				return m, nil
			}
		}
		w := make(chan struct{})
		t.waiters = append(t.waiters, w)
		t.mu.Unlock()

		remain := time.Until(deadline)
		if remain <= 0 {
			return Msg{}, fmt.Errorf("trace: no %s matching predicate within %s", leaf, timeout)
		}
		select {
		case <-w:
		case <-time.After(remain):
			return Msg{}, fmt.Errorf("trace: no %s matching predicate within %s", leaf, timeout)
		}
	}
}
