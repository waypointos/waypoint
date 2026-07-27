package basemap

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestNATSFetcher_RoundTrip(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	require.NoError(t, err)
	go s.Start()
	require.True(t, s.ReadyForConnections(2*time.Second))
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	_, err = nc.Subscribe("waypoint.rover-1.rpc.basemap_tile", func(m *nats.Msg) {
		out, _ := proto.Marshal(&waypointv1.BasemapTileResponse{Found: true, Data: []byte("T"), ContentEncoding: "gzip"})
		_ = m.Respond(out)
	})
	require.NoError(t, err)

	f := NewNATSFetcher(nc, "rover-1", time.Second)
	data, enc, found, err := f.Fetch(5, 1, 2)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []byte("T"), data)
	require.Equal(t, "gzip", enc)
}

func TestNATSFetcher_OfflineWhenNoResponder(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	require.NoError(t, err)
	go s.Start()
	require.True(t, s.ReadyForConnections(2*time.Second))
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// No subscriber for the subject → Request fails → fetcher reports offline.
	f := NewNATSFetcher(nc, "rover-1", 500*time.Millisecond)
	_, _, found, err := f.Fetch(5, 1, 2)
	require.ErrorIs(t, err, errOffline) // handler relies on this sentinel → 204
	require.False(t, found)
}
