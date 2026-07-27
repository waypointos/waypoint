package localauth

import (
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func TestNewBuilder_DefaultUserOnly(t *testing.T) {
	b := NewBuilder()
	users, defaultName := b.Build()
	if len(users) != 1 {
		t.Fatalf("want 1 user, got %d", len(users))
	}
	if users[0].Username != defaultName {
		t.Fatalf("default user mismatch: got %q, defaultName=%q", users[0].Username, defaultName)
	}
	if users[0].Permissions != nil {
		t.Fatalf("default user must have no permissions narrowing; got %+v", users[0].Permissions)
	}
}

func TestNewBuilder_AddsModuleUser(t *testing.T) {
	b := NewBuilder()
	creds, err := b.AddModuleUser("umr", "rover-01", ModulePermissions{
		Publish:   []string{"waypoint.*.module.umr.stats"},
		Subscribe: []string{"waypoint.*.module.umr.health.ready"},
	})
	if err != nil {
		t.Fatalf("AddModuleUser: %v", err)
	}
	if creds.Username == "" || creds.Password == "" {
		t.Fatalf("creds empty: %+v", creds)
	}
	users, _ := b.Build()
	if len(users) != 2 {
		t.Fatalf("want 2 users (default + umr), got %d", len(users))
	}
	var moduleUser *natsserver.User
	for _, u := range users {
		if u.Username == creds.Username {
			moduleUser = u
		}
	}
	if moduleUser == nil {
		t.Fatal("module user not present in build")
	}
	if moduleUser.Permissions == nil {
		t.Fatal("module user must have permissions narrowing")
	}
	if got := moduleUser.Permissions.Publish.Allow; len(got) != 2 {
		t.Fatalf("publish allow len: got %d, want 2 (got %v)", len(got), got)
	}
	if got := moduleUser.Permissions.Publish.Allow[0]; got != "_INBOX.>" {
		t.Fatalf("publish allow[0] = %q, want _INBOX.>", got)
	}
	if !containsString(moduleUser.Permissions.Publish.Allow, "waypoint.rover-01.module.umr.stats") {
		t.Fatalf("publish allow missing waypoint.rover-01.module.umr.stats: %v", moduleUser.Permissions.Publish.Allow)
	}
	if got := moduleUser.Permissions.Subscribe.Allow; len(got) != 2 {
		t.Fatalf("subscribe allow len: got %d, want 2 (got %v)", len(got), got)
	}
	if got := moduleUser.Permissions.Subscribe.Allow[0]; got != "_INBOX.>" {
		t.Fatalf("subscribe allow[0] = %q, want _INBOX.>", got)
	}
	if !containsString(moduleUser.Permissions.Subscribe.Allow, "waypoint.rover-01.module.umr.health.ready") {
		t.Fatalf("subscribe allow missing waypoint.rover-01.module.umr.health.ready: %v", moduleUser.Permissions.Subscribe.Allow)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestAddModuleUser_DuplicateID(t *testing.T) {
	b := NewBuilder()
	if _, err := b.AddModuleUser("umr", "rover-01", ModulePermissions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddModuleUser("umr", "rover-01", ModulePermissions{}); err == nil {
		t.Fatal("want duplicate-id error, got nil")
	}
}

func TestHasUser(t *testing.T) {
	b := NewBuilder()
	if b.HasUser("umr") {
		t.Fatal("HasUser must be false before the module user is added")
	}
	if _, err := b.AddModuleUser("umr", "rover-01", ModulePermissions{}); err != nil {
		t.Fatal(err)
	}
	if !b.HasUser("umr") {
		t.Fatal("HasUser must be true after the module user is added")
	}
}
