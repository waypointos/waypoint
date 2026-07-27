// Package cameraproxy tunnels browser WHEP signaling to the rover via NATS.
// Media (RTP/RTCP) flows browser ↔ TURN ↔ rover; this handler only relays
// the SDP offer/answer dance over `waypoint.<rover>.rpc.camera_offer`.
package cameraproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/router"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// AccessLookup resolves the caller's role on a given rover; an empty string
// means no access (403). Matches the EffectiveRole shape used elsewhere in
// the proxy (db.AccessRepo).
type AccessLookup interface {
	EffectiveRole(ctx context.Context, userID uuid.UUID, roverID string, isAdmin bool) (string, error)
}

// roverConns yields a NATS connection in a given rover's account. cameraproxy
// issues camera_offer RPCs on it so the request reaches the leaf-backed agent.
type roverConns interface {
	Conn(roverID string) (*nats.Conn, error)
}

// Deps configures the camera tunnel handler. Wrap is applied to every
// registered route — production passes RequireUser + RequireCameraAccess;
// tests pass nil to bypass auth.
type Deps struct {
	Conns      roverConns
	RPCTimeout time.Duration
	Wrap       func(http.Handler) http.Handler
}

type Handler struct{ deps Deps }

// New registers the camera tunnel routes onto the typed Router. Routes go
// through RequireUser (session required) and then through Deps.Wrap, which in
// production is RequireCameraAccess. Tests may pass a nil Wrap to skip the
// access check; the session check still runs upstream via the Router.
func New(r *router.Router, d Deps) *Handler {
	h := &Handler{deps: d}
	if h.deps.RPCTimeout == 0 {
		h.deps.RPCTimeout = 5 * time.Second
	}
	wrap := h.deps.Wrap
	if wrap == nil {
		wrap = func(next http.Handler) http.Handler { return next }
	}
	r.RequireUser("POST /rover/{id}/camera/{name}/whep", wrap(http.HandlerFunc(h.postOffer)))
	r.RequireUser("DELETE /rover/{id}/camera/{name}/whep/{session}", wrap(http.HandlerFunc(h.deleteSession)))
	return h
}

// RequireCameraAccess returns a middleware that 403s when the authenticated
// caller has no role on the rover named by `{id}`. RequireUser must run
// upstream so authkit.UserFrom(ctx) is populated.
func RequireCameraAccess(access AccessLookup, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := authkit.UserFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		roverID := r.PathValue("id")
		role, err := access.EffectiveRole(r.Context(), u.ID, roverID, u.IsAdmin)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if role == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) postOffer(w http.ResponseWriter, r *http.Request) {
	roverID := r.PathValue("id")
	camera := r.PathValue("name")
	offer, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	session := randHex(8)
	req := &waypointv1.CameraOfferRequest{Camera: camera, SessionId: session, OfferSdp: string(offer)}
	body, _ := proto.Marshal(req)
	subj := fmt.Sprintf("waypoint.%s.rpc.camera_offer", roverID)

	nc, err := h.deps.Conns.Conn(roverID)
	if err != nil {
		http.Error(w, "rover unreachable: "+err.Error(), http.StatusGatewayTimeout)
		return
	}
	resp, err := nc.Request(subj, body, h.deps.RPCTimeout)
	if err != nil {
		http.Error(w, "rover unreachable: "+err.Error(), http.StatusGatewayTimeout)
		return
	}
	var ans waypointv1.CameraOfferResponse
	if err := proto.Unmarshal(resp.Data, &ans); err != nil {
		http.Error(w, "bad rover response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ans.Error != nil && *ans.Error != "" {
		http.Error(w, *ans.Error, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location",
		fmt.Sprintf("/rover/%s/camera/%s/whep/%s", roverID, camera, session))
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(ans.AnswerSdp))
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	// Phase 3 v1: rely on the agent's PeerConnectionStateChange to clean up
	// dead PCs when the browser tears its PC down. Explicit teardown via
	// rpc.camera_close is a one-task add later.
	w.WriteHeader(http.StatusNoContent)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
