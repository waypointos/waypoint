package natshub

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

func TestHub_StartsAndAcceptsLeafConnect(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{
		Operator:    op,
		ClientPort:  -1,
		LeafWSPort:  -1,
		HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()

	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("server not ready")
	}
	if h.LeafURL() == "" {
		t.Fatal("LeafURL must be set")
	}
}

func TestHub_RevokeAccount_DisconnectsClientAndRejectsRetry(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	h.Server.ReadyForConnections(2 * time.Second)

	roverID := "sim-revoke"
	allowed := []string{"waypoint." + roverID + ".>"}
	accountJWT, accountPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	userJWT, userSeed, err := op.MintRoverUserJWT(accountJWT, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(accountJWT); err != nil {
		t.Fatal(err)
	}

	nc, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(userJWT, string(userSeed)),
		nats.NoReconnect(),
	)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer nc.Close()
	// Force an interaction so the account is fully loaded server-side before
	// the revoke runs (UpdateAccountClaims only acts on loaded accounts).
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	revokedJWT, err := op.MintRevokedAccountJWT(accountPK)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RevokeAccount(accountPK, revokedJWT); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Active connection: poll the server-side connz until it drops, which is
	// the authoritative signal that DisconnectClientByID landed. The client
	// stays optimistic about IsConnected until its next read, which is fine
	// for our threat model — any pub/sub will be rejected.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			cz, _ := h.Server.Connz(&server.ConnzOptions{Account: accountPK})
			t.Fatalf("server did not drop revoked account's clients (connz n=%d)", cz.NumConns)
		default:
		}
		cz, _ := h.Server.Connz(&server.ConnzOptions{Account: accountPK})
		if cz.NumConns == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A fresh connect with the same user JWT must now be rejected — the
	// account JWT is expired and has deny-all permissions.
	nc2, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(userJWT, string(userSeed)),
		nats.Timeout(500*time.Millisecond),
		nats.NoReconnect(),
	)
	if err == nil {
		nc2.Close()
		t.Fatal("expected reconnect to be rejected, but connect succeeded")
	}
}

// TestHub_LeafHandler_AcceptsLeafFromPublicURL drives a real leaf-node
// connection through LeafHandler() to prove the public ingress path
// (Railway → Go HTTP → /leaf reverse proxy → embedded nats-server WS)
// is wired correctly end-to-end.
//
// Failure modes this test catches:
//   (a) /leaf route missing or returning the SPA fallback instead of
//       performing the WebSocket upgrade;
//   (b) StripPrefix path rewriting breaking the leaf-over-WS handshake;
//   (c) nats-server's NoLog default re-enabled, which hides dial
//       failures from operators debugging future regressions.
func TestHub_LeafHandler_AcceptsLeafFromPublicURL(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{
		Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	// Mount LeafHandler on an httptest server with the same /leaf prefix
	// + StripPrefix wrapper the production router uses, so this test
	// exercises the full public-ingress path: HTTP route match →
	// StripPrefix → reverse-proxy → embedded nats-server.
	mux := http.NewServeMux()
	mux.Handle("GET /leaf/", http.StripPrefix("/leaf", h.LeafHandler()))
	publicSrv := httptest.NewServer(mux)
	defer publicSrv.Close()

	// Provision a rover account + user JWT, register the account JWT
	// in the hub's MemAccResolver.
	roverID := "leaf-handler-test"
	allowed := []string{"waypoint." + roverID + ".>"}
	accountJWT, _, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	userJWT, userSeed, err := op.MintRoverUserJWT(accountJWT, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(accountJWT); err != nil {
		t.Fatal(err)
	}

	// Boot an agent-side embedded nats-server with a leaf-node remote
	// pointing at the httptest URL. We build the remote inline rather
	// than via agent/internal/natsleaf so the proxy module stays free
	// of agent-module dependencies (separate go.mod).
	//
	// NATS creds files take the USER JWT (signed by the account nkey),
	// not the account JWT — which is why identity.toml's nats.user_jwt
	// holds what the agent sends here. The hub still needs the account
	// JWT registered (above) so its resolver knows the account exists;
	// the leaf authenticates as a user within it.
	credsPath := writeTestCreds(t, userJWT, string(userSeed))
	defer os.Remove(credsPath)

	leafURL := strings.Replace(publicSrv.URL, "http://", "ws://", 1) + "/leaf"
	u, err := url.Parse(leafURL)
	if err != nil {
		t.Fatal(err)
	}
	agentOpts := &server.Options{
		Host:           "127.0.0.1",
		Port:           -1,
		NoLog:          true,
		NoSigs:         true,
		MaxControlLine: 4096,
	}
	agentOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{
		URLs:        []*url.URL{u},
		Credentials: credsPath,
	}}
	agentOpts.LeafNode.ReconnectInterval = 250 * time.Millisecond
	agentSrv, err := server.NewServer(agentOpts)
	if err != nil {
		t.Fatal(err)
	}
	go agentSrv.Start()
	defer agentSrv.Shutdown()
	if !agentSrv.ReadyForConnections(2 * time.Second) {
		t.Fatal("agent nats not ready")
	}

	// Within a few seconds the leaf should land on the hub.
	deadline := time.After(5 * time.Second)
	for {
		if h.Server.NumLeafNodes() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("leaf node never connected via /leaf; NumLeafNodes=%d", h.Server.NumLeafNodes())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestHub_RestoresRoverAccountsAcrossRestart simulates the full proxy
// lifecycle around a restart: enroll a rover, shut the hub down (and
// discard the operator's in-memory keypair cache to mimic process
// exit), start a fresh hub with the same persisted operator seed,
// restore the account keypair + JWT from the persisted credentials,
// and verify a NATS leaf-node connection using the ORIGINAL user
// JWT/seed still authenticates.
//
// Enforces the persistence contract introduced in migration 0007: a
// regression that drops either operator.RestoreAccountKP or
// hub.RegisterAccount during startup would cause every enrolled rover
// to fail leaf-node authentication after the next proxy restart, even
// though both JWTs remain unchanged on disk.
func TestHub_RestoresRoverAccountsAcrossRestart(t *testing.T) {
	// Phase 1 — Enrollment.
	opSeed, err := freshOperatorSeed(t)
	if err != nil {
		t.Fatal(err)
	}
	op1, err := operator.LoadFromSeed(opSeed)
	if err != nil {
		t.Fatal(err)
	}
	roverID := "restart-test"
	allowed := []string{"waypoint." + roverID + ".>"}
	accountJWT, _, accountSeed, err := op1.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	userJWT, userSeed, err := op1.MintRoverUserJWT(accountJWT, allowed)
	if err != nil {
		t.Fatal(err)
	}

	// Phase 2 — Simulate proxy restart by building a brand-new operator
	// from the same seed and a brand-new hub. Without RestoreAccountKP +
	// hub.RegisterAccount below, the user JWT below would fail auth even
	// though it was minted by a key derived from the SAME operator seed.
	op2, err := operator.LoadFromSeed(opSeed)
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{
		Operator: op2, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	// Phase 3 — Restore. This is what proxy/cmd/waypoint-proxy/main.go's
	// restoreRoverAccounts does for every row in the rovers table on boot.
	if _, err := op2.RestoreAccountKP(accountSeed); err != nil {
		t.Fatalf("restore account kp: %v", err)
	}
	if err := h.RegisterAccount(accountJWT); err != nil {
		t.Fatalf("re-register account: %v", err)
	}

	// Phase 4 — A leaf connect using the ORIGINAL user JWT must succeed.
	// If the test were to regress, NumLeafNodes() would stay at 0.
	mux := http.NewServeMux()
	mux.Handle("GET /leaf/", http.StripPrefix("/leaf", h.LeafHandler()))
	publicSrv := httptest.NewServer(mux)
	defer publicSrv.Close()

	credsPath := writeTestCreds(t, userJWT, string(userSeed))
	defer os.Remove(credsPath)

	leafURL := strings.Replace(publicSrv.URL, "http://", "ws://", 1) + "/leaf"
	u, err := url.Parse(leafURL)
	if err != nil {
		t.Fatal(err)
	}
	agentOpts := &server.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, MaxControlLine: 4096,
	}
	agentOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{
		URLs:        []*url.URL{u},
		Credentials: credsPath,
	}}
	agentOpts.LeafNode.ReconnectInterval = 250 * time.Millisecond
	agentSrv, err := server.NewServer(agentOpts)
	if err != nil {
		t.Fatal(err)
	}
	go agentSrv.Start()
	defer agentSrv.Shutdown()
	if !agentSrv.ReadyForConnections(2 * time.Second) {
		t.Fatal("agent nats not ready")
	}

	deadline := time.After(5 * time.Second)
	for {
		if h.Server.NumLeafNodes() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("leaf node never connected after restart; NumLeafNodes=%d", h.Server.NumLeafNodes())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestHub_RoverPublishReachesSessionsAccountSubscriber proves the
// cross-account bridge: a publish in a rover account on
// waypoint.<id>.infra.heartbeat must reach a subscriber connected as
// the proxy's internal user (sessions account). Without operator's
// Stream Export on the rover account + matching Stream Import on the
// sessions account, NATS enforces account isolation and rovers.last_seen_at
// never updates — symptom: rover stays "offline" in the dashboard even
// while the leaf-node connection is healthy.
func TestHub_RoverPublishReachesSessionsAccountSubscriber(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{
		Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	// Mint + register a rover account with its Stream export, and a
	// matching user JWT to publish under it.
	roverID := "sim-cross-account"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(roverAcctJWT); err != nil {
		t.Fatal(err)
	}

	// Mint + register the sessions account with the rover's import, and
	// an internal user under it to play the role of the heartbeat
	// consumer (which is what the proxy's main.go does in production).
	sessAcctJWT, err := op.MintSessionAccountJWT(
		[]string{"waypoint.>"},
		[]operator.RoverImport{{RoverID: roverID, AccountPubKey: roverAcctPK}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(sessAcctJWT); err != nil {
		t.Fatal(err)
	}
	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}

	// Subscriber: sessions-account internal user.
	subNC, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(internalJWT, string(internalSeed)),
		nats.NoReconnect(),
	)
	if err != nil {
		t.Fatalf("internal subscriber connect: %v", err)
	}
	defer subNC.Close()
	sub, err := subNC.SubscribeSync("waypoint." + roverID + ".infra.heartbeat")
	if err != nil {
		t.Fatal(err)
	}
	if err := subNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Publisher: rover-account user.
	pubNC, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(roverUserJWT, string(roverUserSeed)),
		nats.NoReconnect(),
	)
	if err != nil {
		t.Fatalf("rover publisher connect: %v", err)
	}
	defer pubNC.Close()
	if err := pubNC.Publish("waypoint."+roverID+".infra.heartbeat", []byte("beat")); err != nil {
		t.Fatal(err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("cross-account delivery failed: %v (this is the bug — Stream Export on rover account + matching Import on sessions account must be configured)", err)
	}
	if string(msg.Data) != "beat" {
		t.Fatalf("payload mismatch: %q", msg.Data)
	}
}

// TestHub_RoverModuleStatsReachesSessionsAccountSubscriber proves a module's
// telemetry (waypoint.<id>.module.<mod>.stats) crosses the same full-namespace
// Stream export/import that carries heartbeat/telemetry. This is the pub/sub
// path a proxy-served module panel uses to receive live data (NOT the RPC path
// that doesn't reach a leaf-backed responder). It guards against anyone later
// narrowing the rover account export from waypoint.<id>.> to a subject whitelist.
func TestHub_RoverModuleStatsReachesSessionsAccountSubscriber(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{
		Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-module-stats"
	subject := "waypoint." + roverID + ".module.power-monitor.stats"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(roverAcctJWT); err != nil {
		t.Fatal(err)
	}

	sessAcctJWT, err := op.MintSessionAccountJWT(
		[]string{"waypoint.>"},
		[]operator.RoverImport{{RoverID: roverID, AccountPubKey: roverAcctPK}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(sessAcctJWT); err != nil {
		t.Fatal(err)
	}
	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}

	// Subscriber: sessions-account user (the dashboard connects here).
	subNC, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(internalJWT, string(internalSeed)),
		nats.NoReconnect(),
	)
	if err != nil {
		t.Fatalf("sessions subscriber connect: %v", err)
	}
	defer subNC.Close()
	sub, err := subNC.SubscribeSync(subject)
	if err != nil {
		t.Fatal(err)
	}
	if err := subNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Publisher: rover-account user (the module publishes here on the rover).
	pubNC, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(roverUserJWT, string(roverUserSeed)),
		nats.NoReconnect(),
	)
	if err != nil {
		t.Fatalf("rover publisher connect: %v", err)
	}
	defer pubNC.Close()
	if err := pubNC.Publish(subject, []byte("stats")); err != nil {
		t.Fatal(err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("module stats did not cross the bridge: %v (the rover account export must stay waypoint.<id>.> — a subject whitelist would break proxy module panels)", err)
	}
	if string(msg.Data) != "stats" {
		t.Fatalf("payload mismatch: %q", msg.Data)
	}
}

// freshOperatorSeed mints a fresh operator NKey seed for tests that
// need to simulate a process restart (LoadFromSeed twice with the same
// bytes), without depending on a Mint helper that this package doesn't
// expose. operator.New().Seed() round-trips identically.
func freshOperatorSeed(t *testing.T) ([]byte, error) {
	t.Helper()
	op, err := operator.New()
	if err != nil {
		return nil, err
	}
	return op.Seed()
}

// writeTestCreds mirrors the minimal subset of agent/internal/natsleaf's
// creds-file writer so this package doesn't need a cross-module import.
func writeTestCreds(t *testing.T, jwt, seed string) string {
	t.Helper()
	f, err := os.CreateTemp("", "natshub-leaf-test-*.creds")
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`-----BEGIN NATS USER JWT-----
%s
------END NATS USER JWT------

-----BEGIN USER NKEY SEED-----
%s
------END USER NKEY SEED------
`, jwt, seed)
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		t.Fatal(err)
	}
	return f.Name()
}

func TestHub_RoverLeafCanPubSubInOwnScope(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	h.Server.ReadyForConnections(2 * time.Second)

	roverID := "sim-01"
	allowed := []string{"waypoint." + roverID + ".>"}
	accountJWT, _, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	userJWT, userSeed, err := op.MintRoverUserJWT(accountJWT, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(accountJWT); err != nil {
		t.Fatal(err)
	}

	nc, err := nats.Connect(h.Server.ClientURL(),
		nats.UserJWTAndSeed(userJWT, string(userSeed)),
	)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer nc.Close()

	sub, err := nc.SubscribeSync("waypoint." + roverID + ".telemetry.drive")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("waypoint."+roverID+".telemetry.drive", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("never received: %v", err)
	}
	if string(msg.Data) != "hello" {
		t.Fatalf("payload mismatch: %q", msg.Data)
	}
}

// TestHub_RequestReachesResponderBehindLeaf reproduces the PRODUCTION WHEP
// topology and the fix: the responder (agent camera_offer handler) lives on the
// agent's embedded server, joined to the rover account via a leaf. The proxy
// issues the request from the rover's OWN account (MintRoverInternalUserJWT) so
// it is intra-account — which the leaf carries both ways. A cross-account
// Service import would NOT reach a leaf-backed responder.
func TestHub_RequestReachesResponderBehindLeaf(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := Start(context.Background(), Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1, Logs: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-leaf-rpc"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	// Leaf credential for the agent server (responder side). nil → inherited
	// perms: verifies account DefaultPermissions carry _INBOX.> + Resp.
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(roverAcctJWT); err != nil {
		t.Fatal(err)
	}

	// Public /leaf ingress, mirroring the production router.
	mux := http.NewServeMux()
	mux.Handle("GET /leaf/", http.StripPrefix("/leaf", h.LeafHandler()))
	publicSrv := httptest.NewServer(mux)
	defer publicSrv.Close()

	// Agent-side embedded server joined to the rover account via a leaf.
	credsPath := writeTestCreds(t, roverUserJWT, string(roverUserSeed))
	defer os.Remove(credsPath)
	leafURL := strings.Replace(publicSrv.URL, "http://", "ws://", 1) + "/leaf"
	u, err := url.Parse(leafURL)
	if err != nil {
		t.Fatal(err)
	}
	agentOpts := &server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, MaxControlLine: 4096}
	agentOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{u}, Credentials: credsPath}}
	agentOpts.LeafNode.ReconnectInterval = 250 * time.Millisecond
	agentSrv, err := server.NewServer(agentOpts)
	if err != nil {
		t.Fatal(err)
	}
	go agentSrv.Start()
	defer agentSrv.Shutdown()
	if !agentSrv.ReadyForConnections(2 * time.Second) {
		t.Fatal("agent nats not ready")
	}
	deadline := time.After(5 * time.Second)
	for h.Server.NumLeafNodes() == 0 {
		select {
		case <-deadline:
			t.Fatal("leaf never connected")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Responder on the agent server (no auth, like the agent's camera_offer
	// handler on its embedded NATS).
	agentNC, err := nats.Connect(agentSrv.ClientURL())
	if err != nil {
		t.Fatalf("agent client connect: %v", err)
	}
	defer agentNC.Close()
	subj := "waypoint." + roverID + ".rpc.camera_offer"
	if _, err := agentNC.Subscribe(subj, func(m *nats.Msg) {
		_ = m.Respond([]byte("answer-sdp"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := agentNC.Flush(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let sub interest propagate up the leaf

	// Requester: a user in the rover's OWN account (what the proxy's per-rover
	// connection uses). Intra-account request/reply over the leaf.
	reqUserJWT, reqUserSeed, err := op.MintRoverInternalUserJWT(roverAcctPK, roverID)
	if err != nil {
		t.Fatal(err)
	}
	reqNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(reqUserJWT, string(reqUserSeed)))
	if err != nil {
		t.Fatalf("rover-internal connect: %v", err)
	}
	defer reqNC.Close()

	msg, err := reqNC.Request(subj, []byte("offer-sdp"), 3*time.Second)
	if err != nil {
		t.Fatalf("request to leaf-backed responder failed: %v", err)
	}
	if string(msg.Data) != "answer-sdp" {
		t.Fatalf("reply mismatch: %q", msg.Data)
	}
}
