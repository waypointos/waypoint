package modules

import (
	"context"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// HealthWatcher polls module.<id>.health.<probe> with request/reply. After
// 3 consecutive timeouts the module is marked unhealthy; one good reply
// flips it back. OnChange fires on every transition (not on every tick).
type HealthWatcher struct {
	nc       *natsgo.Conn
	subject  string
	interval time.Duration
	timeout  time.Duration

	OnChange func(healthy bool)

	mu       sync.Mutex
	healthy  bool
	misses   int
	lastSeen time.Time
}

const healthMissThreshold = 3

func NewHealthWatcher(nc *natsgo.Conn, roverID, moduleID, probe string, interval, timeout time.Duration) *HealthWatcher {
	return &HealthWatcher{
		nc:       nc,
		subject:  "waypoint." + roverID + ".module." + moduleID + ".health." + probe,
		interval: interval,
		timeout:  timeout,
	}
}

// Snapshot reports the current observed state. LastSeen is the most recent
// successful reply timestamp.
func (h *HealthWatcher) Snapshot() (healthy bool, lastSeen time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.healthy, h.lastSeen
}

// Run polls until ctx fires.
func (h *HealthWatcher) Run(ctx context.Context) {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.tick(ctx)
		}
	}
}

func (h *HealthWatcher) tick(ctx context.Context) {
	reqCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	_, err := h.nc.RequestWithContext(reqCtx, h.subject, nil)
	h.mu.Lock()
	defer h.mu.Unlock()
	prev := h.healthy
	if err != nil {
		h.misses++
		if h.misses >= healthMissThreshold && h.healthy {
			h.healthy = false
		}
	} else {
		h.misses = 0
		h.lastSeen = time.Now()
		if !h.healthy {
			h.healthy = true
		}
	}
	if h.healthy != prev && h.OnChange != nil {
		go h.OnChange(h.healthy)
	}
}
