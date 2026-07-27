package basemap

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// handleTileRequest decodes a tile request, reads it via the server, and
// returns the marshaled response bytes. Pure and unit-testable.
func handleTileRequest(srv *Server, data []byte) []byte {
	var req waypointv1.BasemapTileRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		out, _ := proto.Marshal(&waypointv1.BasemapTileResponse{Found: false})
		return out
	}
	path := fmt.Sprintf("/%s/%d/%d/%d.mvt", archive, req.Z, req.X, req.Y)
	status, headers, body := srv.Get(context.Background(), path)
	resp := &waypointv1.BasemapTileResponse{}
	if status == http.StatusOK && len(body) > 0 {
		resp.Found = true
		resp.Data = body
		resp.ContentEncoding = headers["Content-Encoding"]
	}
	out, _ := proto.Marshal(resp)
	return out
}

// RPCHandler adapts Server to roverrpc.Handler so the dispatcher can wire it
// onto each rover's own-account connection. Subscribing per-rover (intra-account)
// is required for request/reply to work: the cross-account Stream import the
// sessions account uses for telemetry carries the request but not the reply.
type RPCHandler struct{ Srv *Server }

func (h RPCHandler) Leaf() string             { return "rpc.basemap_tile" }
func (h RPCHandler) Handle(req []byte) []byte { return handleTileRequest(h.Srv, req) }
