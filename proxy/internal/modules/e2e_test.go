package modules

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

// runEmbeddedNATS starts an in-process NATS server on a random port and
// returns its client URL. Mirrors the natshub start sequence so the smoke
// test needs no external nats-server binary.
func runEmbeddedNATS(t *testing.T) string {
	t.Helper()
	s, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1})
	require.NoError(t, err)
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("embedded nats not ready")
	}
	t.Cleanup(s.Shutdown)
	return s.ClientURL()
}

// TestEndToEnd_RegisterPushFetch exercises the full no-rebuild data + trust
// path in one flow, crossing the proxy↔rover boundary over real NATS and
// real HTTP:
//
//	register (cosign verify + Postgres + blob store)
//	  → set-desired
//	  → DesiredPusher publishes on waypoint.<id>.modules.desired
//	  → rover subscriber receives the set
//	  → rover fetches raw_url from the proxy's serve-raw handler
//	  → rover's sha256 gate matches what was registered.
//
// The squashfs build + portablectl attach half is Linux/systemd-only and is
// validated separately on the rover; the .raw bytes are opaque here, so a
// synthetic blob proves the identical proxy/NATS/fetch contract.
func TestEndToEnd_RegisterPushFetch(t *testing.T) {
	ctxBg := context.Background()
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	store := NewDiskBlobStore(t.TempDir())

	const moduleID = "power-monitor"
	const version = "0.1.0"
	const roverID = "rover-01"

	// A real (UI-less) squashfs: ingest opens the image to cache its
	// static tree, so literal fake bytes would fail the superblock read.
	rawBytes, err := os.ReadFile("testdata/noui-0.1.0.raw")
	require.NoError(t, err)
	sum := sha256.Sum256(rawBytes)
	wantSha := hex.EncodeToString(sum[:])

	// Mock module origin (GitHub release artifacts).
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/manifest.json"):
			io.WriteString(w, `{"name":"power-monitor","label":"Power Monitor","version":"0.1.0"}`)
		case strings.HasSuffix(r.URL.Path, ".raw.cosign"):
			io.WriteString(w, `{"fake":"bundle"}`)
		case strings.HasSuffix(r.URL.Path, ".raw"):
			w.Write(rawBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()

	api := NewAPI(repo, store, origin.Client())
	api.verifier = fakeVerifier{}
	api.allowLoopback = true
	blobKey := DeriveBlobKey([]byte("e2e-operator-seed"))
	api = api.WithBlobKey(blobKey)

	// Admin user + seeded rover row (SetDesired has FKs on both).
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(ctxBg, "wk_admin_e2e", "admin-e2e@example.com")
	require.NoError(t, err)
	adminCtx := authkit.WithUser(ctxBg, &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})
	_, err = pool.Exec(ctxBg,
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		roverID, "Test Rover", "pubkey-placeholder", u.ID)
	require.NoError(t, err)

	// Step 1: register the module from its source repo.
	post(t, api.HandleRegister, adminCtx, "/api/admin/modules", map[string]string{
		"source_repo_url": origin.URL,
		"module_id":       moduleID,
	}, nil)

	// Step 2: stand up the proxy's serve-raw endpoint so published raw_urls
	// resolve to the stored blob, and an embedded NATS for the push.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rovers/{roverID}/modules/{moduleID}/{version}", api.HandleServeRaw)
	proxySrv := httptest.NewServer(mux)
	defer proxySrv.Close()

	natsURL := runEmbeddedNATS(t)
	pubConn, err := nats.Connect(natsURL)
	require.NoError(t, err)
	defer pubConn.Close()
	roverConn, err := nats.Connect(natsURL)
	require.NoError(t, err)
	defer roverConn.Close()

	api = api.WithPusher(NewDesiredPusher(repo, stubConns{pubConn}, proxySrv.URL+"/api/rovers", blobKey))

	// Step 3: rover subscribes for desired-state before it's pushed.
	received := make(chan *waypointv1.DesiredModuleSet, 1)
	sub, err := roverConn.Subscribe("waypoint."+roverID+".modules.desired", func(m *nats.Msg) {
		var set waypointv1.DesiredModuleSet
		if err := proto.Unmarshal(m.Data, &set); err != nil {
			t.Errorf("rover: unmarshal desired set: %v", err)
			return
		}
		received <- &set
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, roverConn.Flush())

	// Step 4: admin sets the desired version → triggers the push.
	post(t, api.HandleSetDesired, adminCtx, "/api/admin/rovers/"+roverID+"/modules/"+moduleID,
		map[string]string{"version": version, "config_toml": "[modules_config.power-monitor]\npoll_s = 2\n"},
		map[string]string{"roverID": roverID, "moduleID": moduleID})

	// Step 5: rover receives the set and validates it.
	var set *waypointv1.DesiredModuleSet
	select {
	case set = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("rover never received modules.desired")
	}
	require.Len(t, set.Modules, 1)
	m := set.Modules[0]
	require.Equal(t, moduleID, m.Id)
	require.Equal(t, version, m.Version)
	require.Equal(t, wantSha, m.RawSha256, "published sha256 must match registered blob")
	require.Equal(t, "[modules_config.power-monitor]\npoll_s = 2\n", m.ConfigToml)

	// Step 6: rover fetches the raw and runs its sha256 integrity gate —
	// the same contract agent/internal/modules.Fetcher enforces on-device.
	resp, err := http.Get(m.RawUrl)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	fetched, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, rawBytes, fetched, "served bytes must equal registered bytes")
	gotSum := sha256.Sum256(fetched)
	require.Equal(t, m.RawSha256, hex.EncodeToString(gotSum[:]), "integrity gate must pass")
}

// post invokes a handler directly with a JSON body, optional path values, and
// an auth context, asserting a 200. Calling the handler bypasses the WorkOS
// admin middleware honestly — that's transport, not the logic under test.
func post(t *testing.T, h http.HandlerFunc, ctx context.Context, path string, body, pathVals map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost, path, &buf).WithContext(ctx)
	for k, v := range pathVals {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
