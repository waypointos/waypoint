package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// LocalModules is the agent-local module-management surface exposed over HTTP.
// Implemented by a main-side adapter over *modules.Manager (typed here to avoid
// importing the modules package, which depends on this one).
type LocalModules interface {
	ListModules() []ModuleStatusDTO
	StageModule(ctx context.Context, raw, bundle []byte) (StagePreviewDTO, error)
	ConfirmModule(ctx context.Context, stageID, configTOML string) error
	UninstallModule(ctx context.Context, id string) error
}

type StagePreviewDTO struct {
	StageID     string   `json:"stageId"`
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Label       string   `json:"label"`
	SignerSAN   string   `json:"signerSan"`
	UIKind      string   `json:"uiKind"`
	TabID       string   `json:"tabId"`
	Permissions []string `json:"permissions"`
	Devices     []string `json:"devices"`
	Pinned      bool     `json:"pinned"`
}

type ModuleStatusDTO struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Version string `json:"version"`
	Origin  string `json:"origin"`
}

const maxUploadBytes = 256 << 20 // cap on a module image upload

func (g *Gateway) handleModulesList(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.localModules == nil {
		http.Error(w, "module management unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, g.localModules.ListModules())
}

func (g *Gateway) handleModuleStage(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.localModules == nil {
		http.Error(w, "module management unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "bad upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	raw, err := readFormFile(r, "raw")
	if err != nil {
		http.Error(w, "missing 'raw' file: "+err.Error(), http.StatusBadRequest)
		return
	}
	bundle, err := readFormFile(r, "bundle")
	if err != nil {
		http.Error(w, "missing 'bundle' file: "+err.Error(), http.StatusBadRequest)
		return
	}
	preview, err := g.localModules.StageModule(r.Context(), raw, bundle)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (g *Gateway) handleModuleConfirm(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.localModules == nil {
		http.Error(w, "module management unavailable", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		StageID    string `json:"stageId"`
		ConfigTOML string `json:"configToml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	// All confirm failures (incl. signer-SAN mismatch) return 400 with the
	// message; the dashboard surfaces it verbatim.
	if err := g.localModules.ConfirmModule(r.Context(), in.StageID, in.ConfigTOML); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (g *Gateway) handleModuleUninstall(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.localModules == nil {
		http.Error(w, "module management unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := g.localModules.UninstallModule(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func readFormFile(r *http.Request, field string) ([]byte, error) {
	f, _, err := r.FormFile(field)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxUploadBytes))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
