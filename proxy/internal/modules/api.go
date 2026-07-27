package modules

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/modverify"
)

// moduleIDPattern restricts module IDs to a tight charset so a malicious
// registration cannot escape the on-disk blob path or the on-rover module
// directory layout.
var moduleIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

// verifier is the local interface for cosign verification; satisfied by both
// *modverify.Verifier (prod) and the test fake.
type verifier interface {
	Verify(ctx context.Context, raw, bundleJSON []byte, sanPattern string) (string, error)
}

// lazyVerifier builds a *modverify.Verifier on first use so TUF init does not
// block proxy startup.
type lazyVerifier struct {
	once sync.Once
	v    *modverify.Verifier
	err  error
}

func (l *lazyVerifier) Verify(ctx context.Context, raw, bundleJSON []byte, sanPattern string) (string, error) {
	l.once.Do(func() {
		l.v, l.err = modverify.NewFromTUF()
	})
	if l.err != nil {
		return "", l.err
	}
	return l.v.Verify(ctx, raw, bundleJSON, sanPattern)
}

// tokenBox is satisfied by *crypto.AESGCM; nil means no encryption key is
// configured and token storage is rejected.
type tokenBox interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

// API serves the proxy's admin registry endpoints. It fetches release
// artifacts from a module's source repo, verifies the cosign bundle against
// the repo's GitHub Actions OIDC identity, stores the raw blob, and records
// both the module and its new version in Postgres.
type API struct {
	repo          *db.ModulesRepo
	blobs         BlobStore
	http          *http.Client
	pusher        *DesiredPusher
	verifier      verifier
	allowLoopback bool // test-only; never set in NewAPI
	github        *GitHub
	box           tokenBox
	blobKey       []byte // verifies signed rover blob-download URLs
	staticKey     []byte // mints short-lived static-asset URL tokens
}

func NewAPI(repo *db.ModulesRepo, blobs BlobStore, httpClient *http.Client) *API {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &API{
		repo:     repo,
		blobs:    blobs,
		http:     httpClient,
		verifier: &lazyVerifier{},
	}
}

// WithPusher attaches a DesiredPusher so mutating endpoints can publish the
// updated DesiredModuleSet to the affected rover. Optional: nil-safe.
func (a *API) WithPusher(p *DesiredPusher) *API {
	a.pusher = p
	return a
}

// SetSecrets wires the GitHub client and token box (called from main).
func (a *API) SetSecrets(gh *GitHub, box tokenBox) {
	a.github = gh
	a.box = box
}

// WithBlobKey sets the key that signed rover blob-download URLs are verified
// against; HandleServeRaw rejects everything until one is set.
func (a *API) WithBlobKey(key []byte) *API {
	a.blobKey = key
	return a
}

// WithStaticKey sets the key HandleMintStaticToken mints static-asset URL
// tokens with; minting fails until one is set.
func (a *API) WithStaticKey(key []byte) *API {
	a.staticKey = key
	return a
}

func (a *API) gh() *GitHub {
	if a.github != nil {
		return a.github
	}
	return NewGitHub(a.http)
}

type registerRequest struct {
	SourceRepoURL string `json:"source_repo_url"`
	// ModuleID is optional: the id is taken from the release manifest's "name".
	// When set it must match the manifest, so old clients fail loudly rather
	// than registering under a mismatched id.
	ModuleID       string `json:"module_id"`
	RepoVisibility string `json:"repo_visibility"`
	GithubToken    string `json:"github_token"`
}

// HandleRegister implements POST /api/admin/modules. The module id comes from
// the release manifest's "name" field, so it always matches the `<id>-<ver>.raw`
// asset naming the rest of the pipeline assumes.
func (a *API) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var in registerRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	// SSRF guard: production accepts only https://github.com/<owner>/<repo>.
	// Loopback is permitted only when allowLoopback is set (tests only).
	u, err := url.Parse(in.SourceRepoURL)
	if err != nil || u.Host == "" {
		http.Error(w, "source_repo_url must be a valid URL", http.StatusBadRequest)
		return
	}
	loopback := a.allowLoopback && u.Scheme == "http" && u.Hostname() == "127.0.0.1"
	if !loopback && !(u.Scheme == "https" && u.Host == "github.com") {
		http.Error(w, "source_repo_url must be an https://github.com/<owner>/<repo> URL", http.StatusBadRequest)
		return
	}
	if in.ModuleID != "" && !moduleIDPattern.MatchString(in.ModuleID) {
		http.Error(w, "module_id must match ^[a-z0-9][a-z0-9-]{0,30}$", http.StatusBadRequest)
		return
	}
	user, ok := authkit.UserFrom(r.Context())
	if !ok {
		http.Error(w, "no user in context", http.StatusUnauthorized)
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
	var expectedSAN string
	if loopback {
		expectedSAN = "test" // test-only origins use a fake Verifier; SAN is unused
	} else {
		expectedSAN, err = modverify.ExpectedSANForRepo(in.SourceRepoURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	// Fetch failures below use 424, not 502: CDNs (Cloudflare) replace origin
	// 502s with their own HTML error page, hiding the real message.
	manifestRaw, err := a.fetchBytes(r.Context(), in.SourceRepoURL+"/releases/latest/download/manifest.json")
	if err != nil {
		http.Error(w, "fetch manifest: "+err.Error(), http.StatusFailedDependency)
		return
	}
	var manifest struct {
		Name    string `json:"name"`
		Label   string `json:"label"`
		Version string `json:"version"`
		UI      struct {
			Static *struct {
				Bundle string `json:"bundle"`
			} `json:"static"`
		} `json:"ui"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		http.Error(w, "parse manifest: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	moduleID := manifest.Name
	if !moduleIDPattern.MatchString(moduleID) {
		http.Error(w, fmt.Sprintf("manifest name %q is not a valid module id", moduleID), http.StatusUnprocessableEntity)
		return
	}
	if in.ModuleID != "" && in.ModuleID != moduleID {
		http.Error(w, fmt.Sprintf("module_id %q does not match the release manifest name %q", in.ModuleID, moduleID), http.StatusBadRequest)
		return
	}
	// Enforce the identity pin: a module's signing identity is fixed at first
	// registration and may not be silently re-pointed at a different repo.
	if existing, gerr := a.repo.GetModule(r.Context(), moduleID); gerr == nil {
		if existing.ExpectedIdentitySAN != expectedSAN {
			http.Error(w, "module already registered with a different signing identity; re-pinning is not permitted", http.StatusConflict)
			return
		}
	} else if !errors.Is(gerr, pgx.ErrNoRows) {
		http.Error(w, "lookup module: "+gerr.Error(), http.StatusInternalServerError)
		return
	}
	existing, err := a.repo.ListVersions(r.Context(), moduleID)
	if err != nil {
		http.Error(w, "list versions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var have []string
	for _, v := range existing {
		have = append(have, v.Version)
	}
	if !IsNewerVersion(manifest.Version, have) {
		http.Error(w, "version not newer than current max", http.StatusConflict)
		return
	}
	rawURL := fmt.Sprintf("%s/releases/latest/download/%s-%s.raw", in.SourceRepoURL, moduleID, manifest.Version)
	rawBytes, err := a.fetchBytes(r.Context(), rawURL)
	if err != nil {
		http.Error(w, "fetch raw: "+err.Error(), http.StatusFailedDependency)
		return
	}
	bundleBytes, err := a.fetchBytes(r.Context(), rawURL+".cosign")
	if err != nil {
		http.Error(w, "fetch cosign bundle: "+err.Error(), http.StatusFailedDependency)
		return
	}
	// SAN is pinned via expectedSAN above; the witnessed identity isn't needed here.
	if _, err := a.verifier.Verify(r.Context(), rawBytes, bundleBytes, expectedSAN); err != nil {
		http.Error(w, "verify: "+err.Error(), http.StatusUnauthorized)
		return
	}
	storedPath, err := a.blobs.Put(r.Context(), moduleID, manifest.Version, bytes.NewReader(rawBytes))
	if err != nil {
		http.Error(w, "store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := cacheStaticTree(r.Context(), a.blobs, moduleID, manifest.Version, rawBytes); err != nil {
		http.Error(w, "cache static tree: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	sum := sha256.Sum256(rawBytes)
	if err := a.repo.RegisterModule(r.Context(), db.RegisterModuleInput{
		ModuleID:            moduleID,
		DisplayName:         manifest.Label,
		SourceRepoURL:       in.SourceRepoURL,
		ExpectedIdentitySAN: expectedSAN,
		RegisteredBy:        user.ID,
		RepoVisibility:      in.RepoVisibility,
		GithubTokenEnc:      tokenEnc,
	}); err != nil {
		http.Error(w, "register: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.repo.IngestVersion(r.Context(), db.IngestVersionInput{
		ModuleID:     moduleID,
		Version:      manifest.Version,
		RawPath:      storedPath,
		RawSha256:    hex.EncodeToString(sum[:]),
		ManifestJSON: manifestRaw,
	}); err != nil {
		http.Error(w, "ingest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleList implements GET /api/admin/modules.
func (a *API) HandleList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.repo.ListAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type version struct {
		Version    string `json:"version"`
		IngestedAt string `json:"ingested_at"`
	}
	type module struct {
		ModuleID      string    `json:"module_id"`
		DisplayName   string    `json:"display_name"`
		SourceRepoURL string    `json:"source_repo_url"`
		Versions      []version `json:"versions"`
	}
	out := make([]module, 0, len(rows))
	for _, m := range rows {
		vs := make([]version, 0, len(m.Versions))
		for _, v := range m.Versions {
			vs = append(vs, version{Version: v.Version, IngestedAt: v.IngestedAt.Format("2006-01-02T15:04:05Z")})
		}
		out = append(out, module{
			ModuleID:      m.Module.ModuleID,
			DisplayName:   m.Module.DisplayName,
			SourceRepoURL: m.Module.SourceRepoURL,
			Versions:      vs,
		})
	}
	json.NewEncoder(w).Encode(map[string]any{"modules": out})
}

type setDesiredRequest struct {
	Version    string `json:"version"`
	ConfigTOML string `json:"config_toml"`
}

// HandleSetDesired implements POST /api/admin/rovers/{roverID}/modules/{moduleID}.
// Records the desired version + config for one rover-module pair and, if a
// pusher is wired, publishes the updated DesiredModuleSet to the rover.
func (a *API) HandleSetDesired(w http.ResponseWriter, r *http.Request) {
	roverID := r.PathValue("roverID")
	moduleID := r.PathValue("moduleID")
	var in setDesiredRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	user, ok := authkit.UserFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := a.repo.SetDesired(r.Context(), db.SetDesiredInput{
		RoverID:        roverID,
		ModuleID:       moduleID,
		DesiredVersion: in.Version,
		ConfigTOML:     in.ConfigTOML,
		UpdatedBy:      user.ID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a.pusher != nil {
		_ = a.pusher.Push(r.Context(), roverID)
	}
	w.WriteHeader(http.StatusOK)
}

// HandleUnpin implements DELETE /api/admin/rovers/{roverID}/modules/{moduleID}.
// Drops the pairing and pushes the shrunken desired set; the rover's
// reconciler detaches and stops the module.
func (a *API) HandleUnpin(w http.ResponseWriter, r *http.Request) {
	roverID := r.PathValue("roverID")
	moduleID := r.PathValue("moduleID")
	if err := a.repo.Unpin(r.Context(), roverID, moduleID); err != nil {
		http.Error(w, "unpin: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if a.pusher != nil {
		_ = a.pusher.Push(r.Context(), roverID)
	}
	w.WriteHeader(http.StatusOK)
}

// HandleDeleteModule implements DELETE /api/admin/modules/{moduleID}. Refused
// while any rover still has the module pinned, so a removal is always an
// explicit two-step: unpin everywhere, then delete. Deleting also forgets the
// TOFU identity pin — delete + re-register is the sanctioned way to re-point
// a module id at a different repo.
func (a *API) HandleDeleteModule(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("moduleID")
	rovers, err := a.repo.RoversForModule(r.Context(), moduleID)
	if err != nil {
		http.Error(w, "list rovers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(rovers) > 0 {
		ids := make([]string, 0, len(rovers))
		for _, rm := range rovers {
			ids = append(ids, rm.RoverID)
		}
		http.Error(w, "module is pinned to rovers ("+strings.Join(ids, ", ")+"); unpin it there first", http.StatusConflict)
		return
	}
	versions, err := a.repo.ListVersions(r.Context(), moduleID)
	if err != nil {
		http.Error(w, "list versions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.repo.DeleteModule(r.Context(), moduleID); err != nil {
		http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Registry rows are gone; artifact cleanup is best effort.
	for _, v := range versions {
		if derr := a.blobs.Delete(r.Context(), v.RawPath); derr != nil {
			slog.Warn(fmt.Sprintf("module delete: blob %s: %v", v.RawPath, derr))
		}
		if derr := a.blobs.DeleteStatic(r.Context(), moduleID, v.Version); derr != nil {
			slog.Warn(fmt.Sprintf("module delete: static %s@%s: %v", moduleID, v.Version, derr))
		}
	}
	w.WriteHeader(http.StatusOK)
}

// HandleListDesired implements GET /api/admin/rovers/{roverID}/modules.
// Returns the modules an admin has marked desired for one rover (the proxy's
// intent; actual running state comes from the rover's infra.modules snapshot).
func (a *API) HandleListDesired(w http.ResponseWriter, r *http.Request) {
	roverID := r.PathValue("roverID")
	rows, err := a.repo.DesiredForRover(r.Context(), roverID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type desired struct {
		ModuleID   string `json:"module_id"`
		Version    string `json:"version"`
		ConfigTOML string `json:"config_toml"`
		UpdatedAt  string `json:"updated_at"`
	}
	out := make([]desired, 0, len(rows))
	for _, d := range rows {
		out = append(out, desired{
			ModuleID:   d.ModuleID,
			Version:    d.DesiredVersion,
			ConfigTOML: d.ConfigTOML,
			UpdatedAt:  d.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	json.NewEncoder(w).Encode(map[string]any{"desired": out})
}

// HandleServeRaw serves /api/rovers/{roverID}/modules/{moduleID}/{version}.raw
// to rovers. Auth is the ?sig= HMAC minted by DesiredPusher — rovers hold no
// session, so the route is registered Public and the signature is the gate.
// The rover's sha256 check is the integrity gate.
func (a *API) HandleServeRaw(w http.ResponseWriter, r *http.Request) {
	roverID := r.PathValue("roverID")
	moduleID := r.PathValue("moduleID")
	rawVersion := r.PathValue("version")
	// Route shape is "{version}.raw"; strip the suffix to get the actual version.
	version := strings.TrimSuffix(rawVersion, ".raw")
	if !VerifyBlobSig(a.blobKey, roverID, moduleID, version, r.URL.Query().Get("sig")) {
		http.Error(w, "invalid or missing sig", http.StatusForbidden)
		return
	}
	versions, err := a.repo.ListVersions(r.Context(), moduleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var path string
	for _, v := range versions {
		if v.Version == version {
			path = v.RawPath
			break
		}
	}
	if path == "" {
		http.NotFound(w, r)
		return
	}
	rc, err := a.blobs.Get(r.Context(), path)
	if err != nil {
		http.Error(w, "blob: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := io.Copy(w, rc); err != nil {
		// Headers already flushed; logging-only at this point.
		return
	}
}

// cacheStaticTree extracts the image's whole static UI tree (its dashboard/
// directory) and caches every file for HandleServeStatic. Modules without a
// static root are a no-op.
func cacheStaticTree(ctx context.Context, blobs BlobStore, moduleID, version string, raw []byte) error {
	tree, err := extractStaticTree(raw)
	if err != nil {
		return err
	}
	for rel, data := range tree {
		if err := blobs.PutStatic(ctx, moduleID, version, rel, data); err != nil {
			return err
		}
	}
	return nil
}

// staticContentType maps a static-tree path to its Content-Type. Module
// scripts must carry a JavaScript MIME or browsers refuse to import() them,
// so .js is pinned rather than left to the platform mime table.
func staticContentType(rel string) string {
	if strings.HasSuffix(rel, ".js") || strings.HasSuffix(rel, ".mjs") {
		return "text/javascript; charset=utf-8"
	}
	if ct := mime.TypeByExtension(path.Ext(rel)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// HandleServeStatic implements GET /api/admin/rovers/{roverID}/modules/{moduleID}/static/{path...}.
// Serves the requested file from the static tree of the version the rover
// currently has desired — the same tree the agent serves locally under
// /module/<id>/static/*. Files were extracted from the cosign-verified .raw
// at ingest time.
func (a *API) HandleServeStatic(w http.ResponseWriter, r *http.Request) {
	roverID := r.PathValue("roverID")
	moduleID := r.PathValue("moduleID")
	// Rooted-clean the request path so "..s" cannot address other cache keys.
	rel := strings.TrimPrefix(path.Clean("/"+r.PathValue("path")), "/")
	if rel == "" || rel == "." {
		http.NotFound(w, r)
		return
	}
	rows, err := a.repo.DesiredForRover(r.Context(), roverID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var version string
	for _, d := range rows {
		if d.ModuleID == moduleID {
			version = d.DesiredVersion
			break
		}
	}
	if version == "" {
		slog.Warn(fmt.Sprintf("static serve: %s not desired on rover %s", moduleID, roverID))
		http.NotFound(w, r)
		return
	}
	rc, err := a.blobs.GetStatic(r.Context(), moduleID, version, rel)
	if err != nil {
		// Cache miss: re-extract from the stored raw. Covers versions whose
		// ingest predates static caching and any lost cache entries.
		if rc, err = a.repairStatic(r.Context(), moduleID, version, rel); err != nil {
			slog.Warn(fmt.Sprintf("static serve: repair %s@%s %s: %v", moduleID, version, rel, err))
			http.NotFound(w, r)
			return
		}
	}
	defer rc.Close()
	w.Header().Set("Content-Type", staticContentType(rel))
	if _, err := io.Copy(w, rc); err != nil {
		// Headers already flushed; logging-only at this point.
		return
	}
}

// repairStatic re-extracts a version's whole static tree from its stored raw,
// re-caches it, and returns the requested file.
func (a *API) repairStatic(ctx context.Context, moduleID, version, rel string) (io.ReadCloser, error) {
	versions, err := a.repo.ListVersions(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	var row *db.VersionRow
	for i := range versions {
		if versions[i].Version == version {
			row = &versions[i]
			break
		}
	}
	if row == nil {
		return nil, errors.New("static repair: unknown version")
	}
	rawRC, err := a.blobs.Get(ctx, row.RawPath)
	if err != nil {
		return nil, err
	}
	defer rawRC.Close()
	rawBytes, err := io.ReadAll(rawRC)
	if err != nil {
		return nil, err
	}
	tree, err := extractStaticTree(rawBytes)
	if err != nil {
		return nil, err
	}
	data, ok := tree[rel]
	if !ok {
		return nil, fmt.Errorf("static repair: no %s in image", rel)
	}
	for treeRel, treeData := range tree {
		if perr := a.blobs.PutStatic(ctx, moduleID, version, treeRel, treeData); perr != nil {
			slog.Warn(fmt.Sprintf("static repair: cache %s@%s %s: %v", moduleID, version, treeRel, perr))
		}
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// staticTokenTTL bounds how long a minted static-asset URL stays importable.
// The dashboard imports immediately after minting; the slack covers slow
// networks, not reuse.
const staticTokenTTL = 5 * time.Minute

// HandleMintStaticToken implements GET /api/admin/rovers/{roverID}/modules/{moduleID}/static-token.
// A dynamic import() cannot carry the admin session cookie the way fetch()
// does, so the dashboard trades its session for a short-lived token here and
// appends it to the module-script URL as ?st=.
func (a *API) HandleMintStaticToken(w http.ResponseWriter, r *http.Request) {
	if len(a.staticKey) == 0 {
		http.Error(w, "static tokens not configured", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(staticTokenTTL)
	token := MintStaticToken(a.staticKey, r.PathValue("roverID"), r.PathValue("moduleID"), expires)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":      token,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

type deployRover struct {
	RoverID    string `json:"rover_id"`
	Version    string `json:"version"`
	AutoUpdate bool   `json:"auto_update"`
	// Omitted (nil) keeps the rover's stored config; the config form is the
	// only caller that means to replace it.
	ConfigTOML *string `json:"config_toml"`
}
type deployRequest struct {
	Rovers []deployRover `json:"rovers"`
}

func (a *API) decryptToken(mod *db.ModuleRow) string {
	if len(mod.GithubTokenEnc) == 0 || a.box == nil {
		return ""
	}
	pt, err := a.box.Open(mod.GithubTokenEnc)
	if err != nil {
		return ""
	}
	return string(pt)
}

func (a *API) checker() *CheckUpdater {
	return &CheckUpdater{
		Repo: a.repo, Blobs: a.blobs, GitHub: a.gh(), Verifier: a.verifier,
		Push: func(ctx context.Context, roverID string) error {
			if a.pusher == nil {
				return nil
			}
			return a.pusher.Push(ctx, roverID)
		},
	}
}

func (a *API) HandleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("moduleID")
	mod, err := a.repo.GetModule(r.Context(), moduleID)
	if err != nil {
		http.Error(w, "unknown module", http.StatusNotFound)
		return
	}
	ctx := withToken(r.Context(), a.decryptToken(mod))
	res, err := a.checker().CheckModule(ctx, moduleID)
	if err != nil {
		http.Error(w, "check: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, res)
}

func (a *API) HandleDeploy(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("moduleID")
	user, ok := authkit.UserFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in deployRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	type result struct {
		RoverID string `json:"rover_id"`
		OK      bool   `json:"ok"`
		Error   string `json:"error,omitempty"`
	}
	var results []result
	for _, rv := range in.Rovers {
		var err error
		if rv.ConfigTOML == nil {
			err = a.repo.SetDesiredVersion(r.Context(), rv.RoverID, moduleID, rv.Version, user.ID)
		} else {
			err = a.repo.SetDesired(r.Context(), db.SetDesiredInput{
				RoverID: rv.RoverID, ModuleID: moduleID, DesiredVersion: rv.Version,
				ConfigTOML: *rv.ConfigTOML, UpdatedBy: user.ID,
			})
		}
		if err == nil {
			err = a.repo.SetAutoUpdate(r.Context(), rv.RoverID, moduleID, rv.AutoUpdate)
		}
		if err == nil && a.pusher != nil {
			err = a.pusher.Push(r.Context(), rv.RoverID)
		}
		res := result{RoverID: rv.RoverID, OK: err == nil}
		if err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	writeJSON(w, map[string]any{"results": results})
}

func (a *API) HandleReadme(w http.ResponseWriter, r *http.Request) {
	markdown, err := a.repo.GetReadme(r.Context(), r.PathValue("moduleID"))
	if err != nil {
		http.Error(w, "unknown module", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"markdown": markdown})
}

func (a *API) HandleMarketplace(w http.ResponseWriter, r *http.Request) {
	mods, err := a.repo.ListAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type roverDTO struct {
		RoverID         string `json:"rover_id"`
		DesiredVersion  string `json:"desired_version"`
		AutoUpdate      bool   `json:"auto_update"`
		UpdateAvailable bool   `json:"update_available"`
	}
	type modDTO struct {
		ModuleID       string     `json:"module_id"`
		DisplayName    string     `json:"display_name"`
		SourceRepoURL  string     `json:"source_repo_url"`
		RepoVisibility string     `json:"repo_visibility"`
		LatestVersion  string     `json:"latest_version"`
		HasReadme      bool       `json:"has_readme"`
		Rovers         []roverDTO `json:"rovers"`
	}
	out := struct {
		Modules []modDTO `json:"modules"`
	}{}
	for _, m := range mods {
		latest := ""
		for _, v := range m.Versions {
			if latest == "" || IsNewerVersion(v.Version, []string{latest}) {
				latest = v.Version
			}
		}
		full, _ := a.repo.GetModule(r.Context(), m.Module.ModuleID)
		readme, _ := a.repo.GetReadme(r.Context(), m.Module.ModuleID)
		rovers, _ := a.repo.RoversForModule(r.Context(), m.Module.ModuleID)
		rds := []roverDTO{}
		for _, rm := range rovers {
			rds = append(rds, roverDTO{
				RoverID: rm.RoverID, DesiredVersion: rm.DesiredVersion, AutoUpdate: rm.AutoUpdate,
				UpdateAvailable: latest != "" && rm.DesiredVersion != "" && IsNewerVersion(latest, []string{rm.DesiredVersion}),
			})
		}
		vis := "public"
		if full != nil {
			vis = full.RepoVisibility
		}
		out.Modules = append(out.Modules, modDTO{
			ModuleID: m.Module.ModuleID, DisplayName: m.Module.DisplayName, SourceRepoURL: m.Module.SourceRepoURL,
			RepoVisibility: vis, LatestVersion: latest, HasReadme: readme != "", Rovers: rds,
		})
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) fetchBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}
