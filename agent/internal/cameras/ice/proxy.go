package ice

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// RequestFromProxy asks the proxy for the current TURN config and installs
// it via SetProxyProvided. Best-effort: logs and returns silently on any
// error so callers can carry on with LAN-direct.
func RequestFromProxy(nc *nats.Conn, roverID string, timeout time.Duration) {
	subj := fmt.Sprintf("waypoint.%s.rpc.ice_servers", roverID)
	resp, err := nc.Request(subj, nil, timeout)
	if err != nil {
		slog.Warn(fmt.Sprintf("ice: proxy request failed (continuing LAN-direct): %v", err))
		return
	}
	var is waypointv1.IceServers
	if err := proto.Unmarshal(resp.Data, &is); err != nil {
		slog.Warn(fmt.Sprintf("ice: bad proxy response (continuing LAN-direct): %v", err))
		return
	}
	SetProxyProvided(toWebRTC(is.Servers))
	slog.Info(fmt.Sprintf("ice: %d server(s) installed from proxy", len(is.Servers)))
}

func toWebRTC(in []*waypointv1.IceServer) []webrtc.ICEServer {
	out := make([]webrtc.ICEServer, 0, len(in))
	for _, s := range in {
		srv := webrtc.ICEServer{URLs: append([]string(nil), s.Urls...)}
		if s.Username != nil {
			srv.Username = *s.Username
		}
		if s.Credential != nil {
			srv.Credential = *s.Credential
		}
		out = append(out, srv)
	}
	return out
}
