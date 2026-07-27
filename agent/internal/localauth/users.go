// Package localauth builds the auth config for the agent's embedded NATS
// server. The default user keeps unauthenticated clients (core, WS gateway)
// working through NoAuthUser. Per-module users are added with publish/subscribe
// narrowing derived from each module's manifest.
package localauth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// DefaultUsername is the NATS username NoAuthUser maps to. Clients that do
// not present credentials get this user. It has no permissions narrowing.
const DefaultUsername = "_default"

// ModulePermissions are the per-module subject lists from module.toml,
// translated to fully-qualified subjects (literal rover-id substituted for
// the * wildcard) by AddModuleUser.
type ModulePermissions struct {
	Publish   []string
	Subscribe []string
}

// Credentials are returned to the caller per module so they can be written
// to /run/waypoint/modules/<id>/creds.* for the module process to read.
type Credentials struct {
	Username string
	Password string
}

// Builder accumulates Users and emits the slice + the default username for
// NoAuthUser. Not safe for concurrent use.
type Builder struct {
	users  []*natsserver.User
	byName map[string]struct{}
}

func NewBuilder() *Builder {
	b := &Builder{byName: map[string]struct{}{}}
	b.users = append(b.users, &natsserver.User{Username: DefaultUsername, Password: ""})
	b.byName[DefaultUsername] = struct{}{}
	return b
}

// AddModuleUser adds a per-module user with manifest-derived permissions.
// Subject literals with "*" are expanded to use the rover-id literal:
// "waypoint.*.module.umr.stats" → "waypoint.rover-01.module.umr.stats".
// Returns the random credentials the caller should write to disk for the
// module to read.
func (b *Builder) AddModuleUser(moduleID, roverID string, perms ModulePermissions) (Credentials, error) {
	username := "module-" + moduleID
	if _, dup := b.byName[username]; dup {
		return Credentials{}, fmt.Errorf("localauth: duplicate module id %q", moduleID)
	}
	password, err := randomToken(32)
	if err != nil {
		return Credentials{}, fmt.Errorf("localauth: generate password: %w", err)
	}
	u := &natsserver.User{
		Username:    username,
		Password:    password,
		Permissions: &natsserver.Permissions{},
	}
	// _INBOX.> is required for request/reply (nats.go's auto-generated reply subjects).
	pub := append([]string{"_INBOX.>"}, substituteRoverID(perms.Publish, roverID)...)
	sub := append([]string{"_INBOX.>"}, substituteRoverID(perms.Subscribe, roverID)...)
	u.Permissions.Publish = &natsserver.SubjectPermission{Allow: pub}
	u.Permissions.Subscribe = &natsserver.SubjectPermission{Allow: sub}
	b.users = append(b.users, u)
	b.byName[username] = struct{}{}
	return Credentials{Username: username, Password: password}, nil
}

// HasUser reports whether a per-module user has already been added. Lets callers
// make provisioning idempotent (e.g. skip a module already granted creds at boot).
func (b *Builder) HasUser(moduleID string) bool {
	_, ok := b.byName["module-"+moduleID]
	return ok
}

// Build returns the slice of nats-server users and the username that should
// be passed as NoAuthUser. Safe to call multiple times.
func (b *Builder) Build() ([]*natsserver.User, string) {
	out := make([]*natsserver.User, len(b.users))
	copy(out, b.users)
	return out, DefaultUsername
}

func substituteRoverID(patterns []string, roverID string) []string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = strings.ReplaceAll(p, "*", roverID)
	}
	return out
}

func randomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
