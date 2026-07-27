// Package ice resolves the WebRTC ICE server list for outgoing PeerConnections.
//
// Two sources are merged: environment-provided (CLOUDFLARE_TURN_KEY on the
// rover itself) and proxy-provided (delivered at agent boot via
// rpc.ice_servers, see SetProxyProvided). Either side may be empty; the
// caller treats an empty result as LAN-direct (host candidates only).
package ice

import (
	"os"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

var (
	proxyMu  sync.RWMutex
	proxyIce []webrtc.ICEServer
)

// SetProxyProvided replaces the proxy-side ICE servers. Safe to call from
// any goroutine; subsequent Resolve calls return the merged set.
func SetProxyProvided(s []webrtc.ICEServer) {
	proxyMu.Lock()
	defer proxyMu.Unlock()
	proxyIce = append(proxyIce[:0:0], s...)
}

// Resolve returns the merged ICE server list (env-provided then
// proxy-provided). Empty result = LAN-direct only.
//
// Cloudflare Realtime TURN via CLOUDFLARE_TURN_KEY of the form
//
//	"<turn-key-id>:<turn-key-token>"
//
// (See https://developers.cloudflare.com/realtime/turn/.)
func Resolve() []webrtc.ICEServer {
	out := envProvided()
	proxyMu.RLock()
	out = append(out, proxyIce...)
	proxyMu.RUnlock()
	return out
}

func envProvided() []webrtc.ICEServer {
	key := os.Getenv("CLOUDFLARE_TURN_KEY")
	if key == "" {
		return nil
	}
	idTok := strings.SplitN(key, ":", 2)
	if len(idTok) != 2 {
		return nil
	}
	return []webrtc.ICEServer{{
		URLs:       []string{"turns:turn.cloudflare.com:443?transport=tcp"},
		Username:   idTok[0],
		Credential: idTok[1],
	}}
}
