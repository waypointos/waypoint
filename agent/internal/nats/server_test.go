package nats

import (
	"context"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/waypointos/waypoint/agent/internal/localauth"
)

func TestEmbedded_StartConnectPubSub(t *testing.T) {
	srv, err := StartEmbedded(Options{Port: -1}) // random port
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(srv.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatalf("ready: %v", err)
	}

	nc, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)

	got := make(chan string, 1)
	if _, err := nc.Subscribe("hello", func(m *natsgo.Msg) { got <- string(m.Data) }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Publish("hello", []byte("world")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case s := <-got:
		if s != "world" {
			t.Fatalf("got %q, want world", s)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestStartEmbedded_AppliesAuth(t *testing.T) {
	b := localauth.NewBuilder()
	creds, err := b.AddModuleUser("umr", "rover-test", localauth.ModulePermissions{
		Publish: []string{"waypoint.*.module.umr.stats"},
	})
	if err != nil {
		t.Fatal(err)
	}
	users, defaultUser := b.Build()
	srv, err := StartEmbedded(Options{Port: -1, Users: users, NoAuthUser: defaultUser})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Default user (no creds): connect succeeds, publish to allowed subject succeeds.
	def, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect default: %v", err)
	}
	defer def.Close()
	if err := def.Publish("waypoint.rover-test.module.umr.stats", []byte("x")); err != nil {
		t.Fatalf("default publish: %v", err)
	}

	// Module user: connect with creds, publish to allowed subject succeeds,
	// publish to disallowed subject errors with "permissions violation".
	mod, err := natsgo.Connect(srv.ClientURL(), natsgo.UserInfo(creds.Username, creds.Password))
	if err != nil {
		t.Fatalf("connect module: %v", err)
	}
	defer mod.Close()
	errCh := make(chan error, 2)
	mod.SetErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) { errCh <- err })
	if err := mod.Publish("waypoint.rover-test.module.umr.stats", []byte("x")); err != nil {
		t.Fatalf("module publish allowed: %v", err)
	}
	_ = mod.Publish("waypoint.rover-test.other.subject", []byte("x"))
	mod.Flush()
	select {
	case err := <-errCh:
		if !strings.Contains(err.Error(), "Permissions Violation") {
			t.Fatalf("expected permissions violation, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected permissions-violation error from disallowed publish; got none")
	}
}

// TestSetUsers_HotAddsUserWithoutDroppingNoAuth proves a module installed at
// runtime can be granted NATS creds live: SetUsers must add the new user without
// disconnecting the no-auth client (the C++ core, which connects credential-less
// as NoAuthUser and must not be cycled — rebooting it cycles rover hardware).
func TestSetUsers_HotAddsUserWithoutDroppingNoAuth(t *testing.T) {
	b := localauth.NewBuilder()
	users, defaultUser := b.Build() // just the _default user at first
	srv, err := StartEmbedded(Options{Port: -1, Users: users, NoAuthUser: defaultUser})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Core-like no-auth client connected before the reload.
	core, err := natsgo.Connect(srv.ClientURL(), natsgo.NoReconnect())
	if err != nil {
		t.Fatalf("connect core: %v", err)
	}
	defer core.Close()

	// Hot-add a module user, exactly as a runtime install would.
	creds, err := b.AddModuleUser("x", "rover-test", localauth.ModulePermissions{
		Publish:   []string{"waypoint.*.module.x.>"},
		Subscribe: []string{"waypoint.*.module.x.>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	users, defaultUser = b.Build()
	if err := srv.SetUsers(users, defaultUser); err != nil {
		t.Fatalf("SetUsers: %v", err)
	}

	// The no-auth core connection must have survived the reload.
	if err := core.Publish("waypoint.rover-test.telemetry.x", []byte("alive")); err != nil {
		t.Fatalf("core publish after reload: %v", err)
	}
	if err := core.FlushTimeout(time.Second); err != nil {
		t.Fatalf("core flush after reload: %v", err)
	}
	if !core.IsConnected() {
		t.Fatal("no-auth core connection was dropped by the reload")
	}

	// The freshly-added module user can now connect and publish in its subtree.
	mc, err := natsgo.Connect(srv.ClientURL(), natsgo.UserInfo(creds.Username, creds.Password))
	if err != nil {
		t.Fatalf("connect new module user: %v", err)
	}
	defer mc.Close()
	if err := mc.Publish("waypoint.rover-test.module.x.uplink", []byte("ok")); err != nil {
		t.Fatalf("module publish after reload: %v", err)
	}
	if err := mc.FlushTimeout(time.Second); err != nil {
		t.Fatalf("module flush after reload: %v", err)
	}
}

// TestSetUsers_SequentialReloads covers installing two modules back-to-back:
// each SetUsers must build on the prior one (not reset to the startup set), so
// both users remain valid. Guards the baseline-update in SetUsers.
func TestSetUsers_SequentialReloads(t *testing.T) {
	b := localauth.NewBuilder()
	users, def := b.Build()
	srv, err := StartEmbedded(Options{Port: -1, Users: users, NoAuthUser: def})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	credsA, _ := b.AddModuleUser("a", "rover-test", localauth.ModulePermissions{Publish: []string{"waypoint.*.module.a.>"}})
	users, def = b.Build()
	if err := srv.SetUsers(users, def); err != nil {
		t.Fatalf("first SetUsers: %v", err)
	}
	credsB, _ := b.AddModuleUser("b", "rover-test", localauth.ModulePermissions{Publish: []string{"waypoint.*.module.b.>"}})
	users, def = b.Build()
	if err := srv.SetUsers(users, def); err != nil {
		t.Fatalf("second SetUsers: %v", err)
	}

	// Both module users — added across two reloads — must still authenticate.
	for _, c := range []struct{ user, pass string }{
		{credsA.Username, credsA.Password},
		{credsB.Username, credsB.Password},
	} {
		nc, err := natsgo.Connect(srv.ClientURL(), natsgo.UserInfo(c.user, c.pass))
		if err != nil {
			t.Fatalf("connect %s after sequential reloads: %v", c.user, err)
		}
		nc.Close()
	}
}
