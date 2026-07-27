package operator

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
)

func TestOperator_MintAndVerifyAccountJWT(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(op.PublicKey(), "O") {
		t.Fatalf("operator pubkey must start with O, got %q", op.PublicKey())
	}

	tok, accountPubKey, _, err := op.MintAccountJWT("rover-01", []string{"waypoint.rover-01.>"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwt.DecodeAccountClaims(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if claims.Issuer != op.PublicKey() {
		t.Fatalf("issuer mismatch: %q vs %q", claims.Issuer, op.PublicKey())
	}
	if claims.Subject != accountPubKey {
		t.Fatalf("subject mismatch")
	}
}

func TestOperator_LoadFromSeed(t *testing.T) {
	op1, err := New()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := op1.Seed()
	if err != nil {
		t.Fatal(err)
	}
	op2, err := LoadFromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	if op1.PublicKey() != op2.PublicKey() {
		t.Fatalf("pubkeys differ after reload")
	}
}

func TestOperator_MintSessionUserJWT_ScopedToRoversAndRole(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// User has control on sim-01 and monitor on sim-02; not admin.
	accesses := []SessionAccess{
		{RoverID: "sim-01", Role: "control"},
		{RoverID: "sim-02", Role: "monitor"},
	}

	tok, seed, err := op.MintSessionUserJWT("user-uuid-123", accesses, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) == 0 {
		t.Fatal("seed required")
	}

	claims, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatal(err)
	}

	pubAllow := claims.Permissions.Pub.Allow
	subAllow := claims.Permissions.Sub.Allow

	// sim-01 (control): can pub cmd; can sub the rover's full namespace.
	must(t, contains(pubAllow, "waypoint.sim-01.cmd.>"))
	must(t, contains(subAllow, "waypoint.sim-01.>"))
	// sim-01 (control): can pub the three browser-initiated control RPCs.
	must(t, contains(pubAllow, "waypoint.sim-01.rpc.set_mode"))
	must(t, contains(pubAllow, "waypoint.sim-01.rpc.estop"))
	must(t, contains(pubAllow, "waypoint.sim-01.rpc.recover"))
	// ...but NOT the broad rpc.> wildcard, and not proxy-initiated RPCs.
	must(t, !contains(pubAllow, "waypoint.sim-01.rpc.>"))
	must(t, !contains(pubAllow, "waypoint.sim-01.rpc.camera_offer"))

	// sim-02 (monitor): can sub the rover's full namespace; MUST NOT pub cmd.
	must(t, contains(subAllow, "waypoint.sim-02.>"))
	must(t, !contains(pubAllow, "waypoint.sim-02.cmd.>"))

	// The wildcard sub allow subsumes every per-namespace prefix the
	// dashboard subscribes to — telemetry, event, alert, infra, module,
	// diag, even cmd/rpc echo. Pub permissions stay narrow so monitor
	// users can SEE commands but cannot ISSUE them.
	must(t, !contains(pubAllow, "waypoint.sim-01.>"))
	must(t, !contains(pubAllow, "waypoint.sim-02.>"))

	// Cross-rover isolation.
	must(t, !contains(pubAllow, "waypoint.sim-other.>"))
	must(t, !contains(subAllow, "waypoint.sim-other.>"))

	// No dead system.> subscription: that namespace doesn't exist in
	// protocol/subjects.toml — telemetry.system / event.system / infra.system.*
	// are already covered by the .telemetry.> / .event.> / .infra.> wildcards.
	must(t, !contains(subAllow, "waypoint.sim-01.system.>"))
	must(t, !contains(subAllow, "waypoint.sim-02.system.>"))

	// control can publish into the module subtree (interactive panels),
	// fire-and-forget. Non-admin roles still get NO _INBOX.>: control and
	// monitor stay request/reply-free.
	must(t, contains(pubAllow, "waypoint.sim-01.module.>"))
	must(t, !contains(pubAllow, "_INBOX.>"))
	must(t, !contains(subAllow, "_INBOX.>"))

	// TTL respected.
	if claims.Expires == 0 || claims.Expires-claims.IssuedAt != int64(time.Hour.Seconds()) {
		t.Fatalf("ttl wrong: iat=%d exp=%d", claims.IssuedAt, claims.Expires)
	}
}

func TestOperator_MintSessionUserJWT_AdminGetsAllRoles(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}
	accesses := []SessionAccess{{RoverID: "sim-01", Role: "admin"}}
	tok, _, err := op.MintSessionUserJWT("admin-uuid", accesses, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatal(err)
	}
	must(t, contains(c.Permissions.Pub.Allow, "waypoint.sim-01.rpc.>"))

	// Admin also gets the rover's full namespace on the sub side, including
	// rpc.> for request/reply round-trips and module.> for module data.
	must(t, contains(c.Permissions.Sub.Allow, "waypoint.sim-01.>"))

	// Dead system.> sub permission must not be present.
	must(t, !contains(c.Permissions.Sub.Allow, "waypoint.sim-01.system.>"))

	// Admin needs _INBOX.> in both Pub and Sub for NATS request/reply: nats.go
	// auto-generates reply inboxes under _INBOX.<nuid>, and the dashboard must
	// both publish to and subscribe on those subjects to complete an RPC.
	must(t, contains(c.Permissions.Pub.Allow, "_INBOX.>"))
	must(t, contains(c.Permissions.Sub.Allow, "_INBOX.>"))
}

func TestOperator_SessionAccountPublicKey_StableAndAccountPrefixed(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}
	pk := op.SessionAccountPublicKey()
	if !strings.HasPrefix(pk, "A") {
		t.Fatalf("session account pubkey must start with A, got %q", pk)
	}
	// Stable across calls.
	if op.SessionAccountPublicKey() != pk {
		t.Fatalf("session account pubkey not stable across calls")
	}
}

func TestOperator_MintSessionAccountJWT_RoundTrip(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}

	tok, err := op.MintSessionAccountJWT([]string{"waypoint.>"}, nil)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	claims, err := jwt.DecodeAccountClaims(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Subject is the deterministic sessions account pubkey.
	if claims.Subject != op.SessionAccountPublicKey() {
		t.Fatalf("subject %q != session account pubkey %q",
			claims.Subject, op.SessionAccountPublicKey())
	}

	// Issuer is the operator.
	if claims.Issuer != op.PublicKey() {
		t.Fatalf("issuer %q != operator pubkey %q", claims.Issuer, op.PublicKey())
	}

	// Account-default permissions cover waypoint.>.
	must(t, contains(claims.DefaultPermissions.Pub.Allow, "waypoint.>"))
	must(t, contains(claims.DefaultPermissions.Sub.Allow, "waypoint.>"))
}

func TestOperator_LoadFromSeed_PreservesSessionAccount(t *testing.T) {
	op1, err := New()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := op1.Seed()
	if err != nil {
		t.Fatal(err)
	}
	op2, err := LoadFromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	// Same operator seed must deterministically produce the same session account
	// pubkey, otherwise restarting the proxy would invalidate all session JWTs
	// the embedded NATS server has trusted.
	if op1.SessionAccountPublicKey() != op2.SessionAccountPublicKey() {
		t.Fatalf("session account pubkey not stable across LoadFromSeed: %q vs %q",
			op1.SessionAccountPublicKey(), op2.SessionAccountPublicKey())
	}
}

func TestSessionUserJWT_ControlCanPublishModules(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}
	accesses := []SessionAccess{{RoverID: "rover-01", Role: "control"}}
	tok, _, err := op.MintSessionUserJWT("user-1", accesses, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	uc, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatal(err)
	}
	must(t, contains(uc.Permissions.Pub.Allow, "waypoint.rover-01.module.>"))
	// Publish-only: control stays request/reply-free (no _INBOX.>).
	must(t, !contains(uc.Permissions.Pub.Allow, "_INBOX.>"))
}

func TestSessionUserJWT_AdminCanPublishModules(t *testing.T) {
	op, err := New()
	if err != nil {
		t.Fatal(err)
	}
	accesses := []SessionAccess{{RoverID: "rover-01", Role: "admin"}}
	tok, _, err := op.MintSessionUserJWT("admin-1", accesses, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	uc, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatal(err)
	}
	must(t, contains(uc.Permissions.Pub.Allow, "waypoint.rover-01.module.>"))
}

func contains(list jwt.StringList, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func must(t *testing.T, cond bool) {
	t.Helper()
	if !cond {
		t.Fatal("assertion failed")
	}
}
