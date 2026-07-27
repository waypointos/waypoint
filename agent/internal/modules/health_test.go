package modules

import (
	"context"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/waypointos/waypoint/agent/internal/nats"
)

func TestHealthWatcher_HealthyThenTimeout(t *testing.T) {
	srv, err := nats.StartEmbedded(nats.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Fake module: replies to health.ready until we close it.
	mod, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	subj := "waypoint.r1.module.umr.health.ready"
	sub, err := mod.Subscribe(subj, func(m *natsgo.Msg) {
		_ = m.Respond([]byte("ok"))
	})
	if err != nil {
		t.Fatal(err)
	}

	// Watcher: client connection from agent's perspective.
	cli, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	hw := NewHealthWatcher(cli, "r1", "umr", "ready", 50*time.Millisecond, 25*time.Millisecond)

	var mu sync.Mutex
	var states []bool
	hw.OnChange = func(healthy bool) {
		mu.Lock()
		defer mu.Unlock()
		states = append(states, healthy)
	}

	go hw.Run(ctx)
	time.Sleep(200 * time.Millisecond) // let it become healthy

	_ = sub.Unsubscribe()              // simulate module dying
	time.Sleep(500 * time.Millisecond) // ≥3 timeouts → unhealthy

	mu.Lock()
	defer mu.Unlock()
	if len(states) < 2 {
		t.Fatalf("want ≥2 state transitions, got: %v", states)
	}
	if !states[0] {
		t.Fatalf("first transition should be healthy=true, got: %v", states)
	}
	if states[len(states)-1] {
		t.Fatalf("last transition should be healthy=false, got: %v", states)
	}
}
