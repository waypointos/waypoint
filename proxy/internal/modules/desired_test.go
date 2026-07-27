package modules

import (
	"context"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

type stubConns struct{ nc *nats.Conn }

func (s stubConns) Conn(string) (*nats.Conn, error) { return s.nc, nil }

func TestDesiredPusher_PublishesPerRoverSet(t *testing.T) {
	nc, shutdown := startTestNATS(t)
	defer shutdown()

	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin", "admin@example.com")
	require.NoError(t, err)

	// Seed a rover row directly.
	_, err = pool.Exec(context.Background(),
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		"rover-01", "Test", "pubkey-placeholder", u.ID)
	require.NoError(t, err)

	require.NoError(t, repo.RegisterModule(context.Background(), db.RegisterModuleInput{
		ModuleID: "x", DisplayName: "X", SourceRepoURL: "https://e/x",
		ExpectedIdentitySAN: `^https://github\.com/e/x/\.github/workflows/release\.yml@refs/tags/v.+$`,
		RegisteredBy:        u.ID,
	}))
	require.NoError(t, repo.IngestVersion(context.Background(), db.IngestVersionInput{
		ModuleID: "x", Version: "0.1.0",
		RawPath: "/blob/x-0.1.0.raw", RawSha256: "abc", ManifestJSON: []byte(`{}`),
	}))
	require.NoError(t, repo.SetDesired(context.Background(), db.SetDesiredInput{
		RoverID: "rover-01", ModuleID: "x", DesiredVersion: "0.1.0",
		ConfigTOML: "", UpdatedBy: u.ID,
	}))

	received := make(chan []byte, 1)
	sub, err := nc.Subscribe("waypoint.rover-01.modules.desired", func(m *nats.Msg) {
		received <- m.Data
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	blobKey := DeriveBlobKey([]byte("test-operator-seed"))
	p := NewDesiredPusher(repo, stubConns{nc}, "https://proxy.example/api/rovers", blobKey)
	require.NoError(t, p.Push(context.Background(), "rover-01"))

	select {
	case data := <-received:
		var got waypointv1.DesiredModuleSet
		require.NoError(t, proto.Unmarshal(data, &got))
		require.Len(t, got.Modules, 1)
		require.Equal(t, "0.1.0", got.Modules[0].Version)
		require.Contains(t, got.Modules[0].RawUrl, "rover-01")
		require.Contains(t, got.Modules[0].RawUrl, "0.1.0.raw")
		_, sig, found := strings.Cut(got.Modules[0].RawUrl, "?sig=")
		require.True(t, found, "raw_url must carry a download signature")
		require.True(t, VerifyBlobSig(blobKey, "rover-01", "x", "0.1.0", sig))
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive desired set within 2s")
	}
}

// startTestNATS spins up an in-memory NATS server. Same pattern as
// proxy/internal/alerts/mirror_test.go.
func startTestNATS(t *testing.T) (*nats.Conn, func()) {
	t.Helper()
	s, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, MaxControlLine: 4096,
	})
	require.NoError(t, err)
	go s.Start()
	if !s.ReadyForConnections(2 * time.Second) {
		s.Shutdown()
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	require.NoError(t, err)
	return nc, func() { nc.Close(); s.Shutdown() }
}
