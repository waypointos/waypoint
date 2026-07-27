package cameraproxy

import (
	"os"
	"strings"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// iceServersReply builds the protobuf reply bytes for an rpc.ice_servers
// request from the proxy's current TURN environment. With CLOUDFLARE_TURN_KEY
// unset, the response carries an empty server list (agents fall back to
// LAN-direct).
func iceServersReply() []byte {
	resp := &waypointv1.IceServers{}
	for _, s := range iceServersFromEnv() {
		username, credential := s.username, s.credential
		resp.Servers = append(resp.Servers, &waypointv1.IceServer{
			Urls:       s.urls,
			Username:   &username,
			Credential: &credential,
		})
	}
	body, _ := proto.Marshal(resp)
	return body
}

// IceServersHandler adapts the ice_servers response to roverrpc.Handler so the
// dispatcher subscribes it on each rover's own-account connection. Required so
// the agent's nc.Request("...rpc.ice_servers") actually gets a reply back: the
// cross-account Stream import the sessions account uses for telemetry can carry
// the request but not the reply.
type IceServersHandler struct{}

func (IceServersHandler) Leaf() string           { return "rpc.ice_servers" }
func (IceServersHandler) Handle(_ []byte) []byte { return iceServersReply() }

// envServer is a lightweight struct mirroring webrtc.ICEServer for the
// fields we care about, without taking a dependency on pion in the proxy.
type envServer struct {
	urls       []string
	username   string
	credential string
}

func iceServersFromEnv() []envServer {
	key := os.Getenv("CLOUDFLARE_TURN_KEY")
	if key == "" {
		return nil
	}
	idTok := strings.SplitN(key, ":", 2)
	if len(idTok) != 2 {
		return nil
	}
	return []envServer{{
		urls:       []string{"turns:turn.cloudflare.com:443?transport=tcp"},
		username:   idTok[0],
		credential: idTok[1],
	}}
}
