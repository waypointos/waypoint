package gateway

import (
	"net/http"

	"github.com/waypointos/waypoint/agent/internal/recorder"
)

// EpisodeStore is implemented by recorder.Recorder; faked in tests.
type EpisodeStore interface {
	List() ([]*recorder.Sidecar, error)
	Path(id string) (string, error)
	Delete(id string) error
}

// SetEpisodes wires the episode store; nil leaves the routes returning 503.
func (g *Gateway) SetEpisodes(s EpisodeStore) { g.episodes = s }

func (g *Gateway) handleEpisodesList(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.episodes == nil {
		http.Error(w, "recorder unavailable", http.StatusServiceUnavailable)
		return
	}
	scs, err := g.episodes.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scs == nil {
		scs = []*recorder.Sidecar{}
	}
	writeJSON(w, http.StatusOK, scs)
}

func (g *Gateway) handleEpisodeDownload(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.episodes == nil {
		http.Error(w, "recorder unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	p, err := g.episodes.Path(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.mcap"`)
	http.ServeFile(w, r, p)
}

func (g *Gateway) handleEpisodeDelete(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.episodes == nil {
		http.Error(w, "recorder unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := g.episodes.Delete(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
