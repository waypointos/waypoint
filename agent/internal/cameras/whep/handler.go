package whep

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/pion/webrtc/v4"
	"github.com/waypointos/waypoint/agent/internal/cameras"
)

type Resolver struct {
	Cameras func(name string) (*cameras.Camera, bool)
}

type Handler struct{ res Resolver }

func New(mux *http.ServeMux, res Resolver) *Handler {
	h := &Handler{res: res}
	mux.HandleFunc("POST /camera/{name}/whep", h.postOffer)
	mux.HandleFunc("DELETE /camera/{name}/whep/{session}", h.deleteSession)
	return h
}

func (h *Handler) postOffer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cam, ok := h.res.Cameras(name)
	if !ok {
		http.Error(w, "no such camera", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	session := randHex(8)
	answer, err := cam.NewSubscriber(r.Context(), session,
		webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: string(body)},
	)
	if err != nil {
		http.Error(w, "subscribe: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/camera/"+name+"/whep/"+session)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(answer.SDP))
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	session := r.PathValue("session")
	cam, ok := h.res.Cameras(name)
	if !ok {
		http.Error(w, "no such camera", http.StatusNotFound)
		return
	}
	cam.EndSubscriber(session)
	w.WriteHeader(http.StatusNoContent)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
