package basemap

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// NATSFetcher requests tiles from the proxy over the rover's NATS connection.
// The proxy answers waypoint.<rover>.rpc.basemap_tile per-rover on the rover's
// own-account connection (see proxy/internal/roverrpc): keeping the responder
// intra-account is what lets the reply travel back to a leaf-backed publisher.
type NATSFetcher struct {
	nc      *nats.Conn
	roverID string
	timeout time.Duration
}

func NewNATSFetcher(nc *nats.Conn, roverID string, timeout time.Duration) *NATSFetcher {
	return &NATSFetcher{nc: nc, roverID: roverID, timeout: timeout}
}

func (f *NATSFetcher) Fetch(z, x, y uint32) ([]byte, string, bool, error) {
	req, err := proto.Marshal(&waypointv1.BasemapTileRequest{Z: z, X: x, Y: y})
	if err != nil {
		return nil, "", false, err
	}
	subj := fmt.Sprintf("waypoint.%s.rpc.basemap_tile", f.roverID)
	msg, err := f.nc.Request(subj, req, f.timeout)
	if err != nil {
		return nil, "", false, errOffline // no link / no responder → treat as offline
	}
	var resp waypointv1.BasemapTileResponse
	if err := proto.Unmarshal(msg.Data, &resp); err != nil {
		return nil, "", false, err
	}
	return resp.Data, resp.ContentEncoding, resp.Found, nil
}
