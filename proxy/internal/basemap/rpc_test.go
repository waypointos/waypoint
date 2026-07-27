package basemap

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestHandleTileRequest_HitAndMiss(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	srv, err := New(dir)
	require.NoError(t, err)
	defer srv.Close()

	hit, _ := proto.Marshal(&waypointv1.BasemapTileRequest{Z: 0, X: 0, Y: 0})
	var resp waypointv1.BasemapTileResponse
	require.NoError(t, proto.Unmarshal(handleTileRequest(srv, hit), &resp))
	require.True(t, resp.Found)
	require.NotEmpty(t, resp.Data)

	miss, _ := proto.Marshal(&waypointv1.BasemapTileRequest{Z: 5, X: 0, Y: 0})
	var mresp waypointv1.BasemapTileResponse
	require.NoError(t, proto.Unmarshal(handleTileRequest(srv, miss), &mresp))
	require.False(t, mresp.Found)
}
