package images

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/modules"
)

type tokenBox interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

type API struct {
	repo   *db.ReleasesRepo
	rovers *db.RoversRepo
	github *modules.GitHub
	box    tokenBox
}

func NewAPI(repo *db.ReleasesRepo, rovers *db.RoversRepo, gh *modules.GitHub, box tokenBox) *API {
	if gh == nil {
		gh = modules.NewGitHub(nil)
	}
	return &API{repo: repo, rovers: rovers, github: gh, box: box}
}

// channelFor infers a rover's release channel from its current image version.
func channelFor(version string) string {
	if strings.Contains(version, "-dev") {
		return "dev"
	}
	return "prod"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type registerSourceRequest struct {
	RepoURL        string `json:"repo_url"`
	Channel        string `json:"channel"`
	RepoVisibility string `json:"repo_visibility"`
	GithubToken    string `json:"github_token"`
}

func (a *API) HandleRegisterSource(w http.ResponseWriter, r *http.Request) {
	user, ok := authkit.UserFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in registerSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if _, _, err := modules.ParseOwnerRepo(in.RepoURL); err != nil {
		http.Error(w, "repo_url must be an https://github.com/<owner>/<repo> URL", http.StatusBadRequest)
		return
	}
	if in.Channel != "prod" && in.Channel != "dev" {
		http.Error(w, "channel must be prod or dev", http.StatusBadRequest)
		return
	}
	var tokenEnc []byte
	if in.GithubToken != "" {
		if a.box == nil {
			http.Error(w, "token storage not configured", http.StatusBadRequest)
			return
		}
		sealed, err := a.box.Seal([]byte(in.GithubToken))
		if err != nil {
			http.Error(w, "seal token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tokenEnc = sealed
	}
	if err := a.repo.RegisterSource(r.Context(), db.RegisterSourceInput{
		RepoURL: in.RepoURL, Channel: in.Channel, RepoVisibility: in.RepoVisibility,
		GithubTokenEnc: tokenEnc, RegisteredBy: user.ID,
	}); err != nil {
		http.Error(w, "register: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) HandleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	chk := &ReleaseChecker{
		Repo: a.repo, GitHub: a.github,
		DecryptToken: func(src db.ImageSourceRow) string {
			if len(src.GithubTokenEnc) == 0 || a.box == nil {
				return ""
			}
			pt, err := a.box.Open(src.GithubTokenEnc)
			if err != nil {
				return ""
			}
			return string(pt)
		},
	}
	n, err := chk.CheckAll(r.Context())
	if err != nil {
		http.Error(w, "check: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]int{"ingested": n})
}

func (a *API) HandleReleasesFleet(w http.ResponseWriter, r *http.Request) {
	rovers, err := a.rovers.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	latestByChannel := map[string]*db.ImageReleaseRow{}
	get := func(ctx context.Context, ch string) *db.ImageReleaseRow {
		if v, ok := latestByChannel[ch]; ok {
			return v
		}
		v, _ := a.repo.LatestRelease(ctx, ch)
		latestByChannel[ch] = v
		return v
	}

	type roverDTO struct {
		RoverID         string `json:"rover_id"`
		Name            string `json:"name"`
		CurrentVersion  string `json:"current_version"`
		Channel         string `json:"channel"`
		LatestVersion   string `json:"latest_version"`
		UpdateAvailable bool   `json:"update_available"`
		SwuURL          string `json:"swu_url"`
		SwuSha256       string `json:"swu_sha256"`
		ReleaseNotesMD  string `json:"release_notes_md"`
		ReleaseHTMLURL  string `json:"release_html_url"`
	}
	out := struct {
		Rovers []roverDTO `json:"rovers"`
	}{Rovers: []roverDTO{}}

	for _, rv := range rovers {
		cur := ""
		if rv.ImageVersion != nil {
			cur = *rv.ImageVersion
		}
		ch := channelFor(cur)
		d := roverDTO{RoverID: rv.ID, Name: rv.Name, CurrentVersion: cur, Channel: ch}
		if latest := get(r.Context(), ch); latest != nil {
			d.LatestVersion = latest.Version
			d.SwuURL = latest.SwuURL
			d.SwuSha256 = latest.SwuSha256
			d.ReleaseNotesMD = latest.ReleaseNotesMD
			d.ReleaseHTMLURL = latest.ReleaseHTMLURL
			d.UpdateAvailable = cur != "" && isNewer(latest.Version, cur)
		}
		out.Rovers = append(out.Rovers, d)
	}
	writeJSON(w, out)
}

// isNewer reports whether a is a newer semver than b. Reuses the exported
// helper from db so ordering matches LatestRelease.
func isNewer(a, b string) bool { return db.IsNewerVersionSemver(a, b) }
