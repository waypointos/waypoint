package modules

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

// newTestPool spins up a Postgres testcontainer and runs the migrations.
// Duplicated from proxy/internal/db/repo_test.go (which keeps the helper
// package-private) to avoid widening db's exported surface for tests.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	c, err := pgcontainer.RunContainer(ctx,
		pgcontainer.WithDatabase("waypoint"),
		pgcontainer.WithUsername("waypoint"),
		pgcontainer.WithPassword("dev"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := pool.Ping(ctx); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

type fakeVerifier struct{ err error }

func (f fakeVerifier) Verify(context.Context, []byte, []byte, string) (string, error) {
	return "", f.err
}

func TestAPI_RegisterFromRepo_Cosign(t *testing.T) {
	pool := newTestPool(t)
	// A real (UI-less) squashfs: ingest now always opens the image to cache
	// its static tree, so literal fake bytes would fail the superblock read.
	raw, err := os.ReadFile("testdata/noui-0.1.0.raw")
	require.NoError(t, err)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/manifest.json"):
			io.WriteString(w, `{"name":"power-monitor","label":"Power Monitor","version":"0.1.0"}`)
		case strings.HasSuffix(r.URL.Path, ".raw.cosign"):
			io.WriteString(w, `{"fake":"bundle"}`)
		case strings.HasSuffix(r.URL.Path, ".raw"):
			w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()

	api := NewAPI(db.NewModulesRepo(pool), NewDiskBlobStore(t.TempDir()), origin.Client())
	api.verifier = fakeVerifier{}
	api.allowLoopback = true
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin", "admin@example.com")
	require.NoError(t, err)
	ctx := authkit.WithUser(context.Background(), &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})
	// No module_id in the request: the id is derived from the manifest name.
	body, _ := json.Marshal(map[string]string{"source_repo_url": origin.URL})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rows, _ := api.repo.ListVersions(context.Background(), "power-monitor")
	require.Len(t, rows, 1)

	// Re-registering the same version (0.1.0) must be rejected as not newer.
	req2 := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body)).WithContext(ctx)
	rec2 := httptest.NewRecorder()
	api.HandleRegister(rec2, req2)
	require.Equal(t, http.StatusConflict, rec2.Code)
}

func TestAPI_RegisterRejectsMismatchedModuleID(t *testing.T) {
	pool := newTestPool(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/manifest.json") {
			io.WriteString(w, `{"name":"so100","label":"Arm","version":"0.1.0"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer origin.Close()
	api := NewAPI(db.NewModulesRepo(pool), NewDiskBlobStore(t.TempDir()), origin.Client())
	api.verifier = fakeVerifier{}
	api.allowLoopback = true
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin_mismatch", "mismatch@example.com")
	require.NoError(t, err)
	ctx := authkit.WithUser(context.Background(), &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})
	body, _ := json.Marshal(map[string]string{"source_repo_url": origin.URL, "module_id": "waypoint-so100"})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `does not match the release manifest name "so100"`)
	rows, _ := api.repo.ListVersions(context.Background(), "so100")
	require.Len(t, rows, 0)
}

func TestAPI_RegisterRejectsNonHTTPS(t *testing.T) {
	pool := newTestPool(t)
	api := NewAPI(db.NewModulesRepo(pool), NewDiskBlobStore(t.TempDir()), http.DefaultClient)
	body, _ := json.Marshal(map[string]string{"source_repo_url": "file:///etc/passwd", "module_id": "x"})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAPI_RegisterRejectsNonGitHub(t *testing.T) {
	pool := newTestPool(t)
	api := NewAPI(db.NewModulesRepo(pool), NewDiskBlobStore(t.TempDir()), http.DefaultClient)
	body, _ := json.Marshal(map[string]string{"source_repo_url": "https://evil.example/x", "module_id": "x"})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAPI_RegisterRejectsBadModuleID(t *testing.T) {
	pool := newTestPool(t)
	api := NewAPI(db.NewModulesRepo(pool), NewDiskBlobStore(t.TempDir()), http.DefaultClient)
	body, _ := json.Marshal(map[string]string{"source_repo_url": "https://github.com/x/y", "module_id": "../etc"})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAPI_HandleServeRaw(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	store := NewDiskBlobStore(t.TempDir())

	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin_serve", "admin-serve@example.com")
	require.NoError(t, err)

	require.NoError(t, repo.RegisterModule(context.Background(), db.RegisterModuleInput{
		ModuleID:            "raw-test",
		DisplayName:         "Raw Test",
		SourceRepoURL:       "https://github.com/example/raw-test",
		ExpectedIdentitySAN: `^https://github\.com/example/raw-test/\.github/workflows/release\.yml@refs/tags/v.+$`,
		RegisteredBy:        u.ID,
	}))

	rawBytes := []byte("hello squashfs world")
	storedPath, err := store.Put(context.Background(), "raw-test", "0.1.0", bytes.NewReader(rawBytes))
	require.NoError(t, err)
	sum := sha256.Sum256(rawBytes)
	require.NoError(t, repo.IngestVersion(context.Background(), db.IngestVersionInput{
		ModuleID:     "raw-test",
		Version:      "0.1.0",
		RawPath:      storedPath,
		RawSha256:    hex.EncodeToString(sum[:]),
		ManifestJSON: []byte(`{"name":"raw-test","label":"Raw Test","version":"0.1.0"}`),
	}))

	blobKey := DeriveBlobKey([]byte("serve-raw-seed"))
	api := NewAPI(repo, store, http.DefaultClient).WithBlobKey(blobKey)

	sig := signBlobPath(blobKey, "rover-1", "raw-test", "0.1.0")
	req := httptest.NewRequest("GET", "/api/rovers/rover-1/modules/raw-test/0.1.0.raw?sig="+sig, nil)
	req.SetPathValue("roverID", "rover-1")
	req.SetPathValue("moduleID", "raw-test")
	req.SetPathValue("version", "0.1.0.raw")
	rec := httptest.NewRecorder()
	api.HandleServeRaw(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, rawBytes, rec.Body.Bytes())

	// Missing signature → 403; the route is public at the router.
	reqNoSig := httptest.NewRequest("GET", "/api/rovers/rover-1/modules/raw-test/0.1.0.raw", nil)
	reqNoSig.SetPathValue("roverID", "rover-1")
	reqNoSig.SetPathValue("moduleID", "raw-test")
	reqNoSig.SetPathValue("version", "0.1.0.raw")
	recNoSig := httptest.NewRecorder()
	api.HandleServeRaw(recNoSig, reqNoSig)
	require.Equal(t, http.StatusForbidden, recNoSig.Code)

	// Unknown version (validly signed) → 404.
	sig999 := signBlobPath(blobKey, "rover-1", "raw-test", "9.9.9")
	req404 := httptest.NewRequest("GET", "/api/rovers/rover-1/modules/raw-test/9.9.9.raw?sig="+sig999, nil)
	req404.SetPathValue("roverID", "rover-1")
	req404.SetPathValue("moduleID", "raw-test")
	req404.SetPathValue("version", "9.9.9.raw")
	rec404 := httptest.NewRecorder()
	api.HandleServeRaw(rec404, req404)
	require.Equal(t, http.StatusNotFound, rec404.Code)
}

func TestAPI_RegisterRejectsIdentityChange(t *testing.T) {
	pool := newTestPool(t)
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin_pin", "pin@example.com")
	require.NoError(t, err)
	repo := db.NewModulesRepo(pool)
	// Pre-pin module "x" to a specific identity.
	require.NoError(t, repo.RegisterModule(context.Background(), db.RegisterModuleInput{
		ModuleID: "x", DisplayName: "X", SourceRepoURL: "https://github.com/orig/x",
		ExpectedIdentitySAN: "^https://github.com/orig/x/.+$", RegisteredBy: u.ID,
	}))

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/manifest.json"):
			io.WriteString(w, `{"name":"x","label":"X","version":"0.2.0"}`)
		case strings.HasSuffix(r.URL.Path, ".raw.cosign"):
			io.WriteString(w, `{"fake":"bundle"}`)
		case strings.HasSuffix(r.URL.Path, ".raw"):
			w.Write([]byte("raw"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()
	api := NewAPI(repo, NewDiskBlobStore(t.TempDir()), origin.Client())
	api.verifier = fakeVerifier{}
	api.allowLoopback = true // loopback origin derives SAN "test" != the pinned SAN
	ctx := authkit.WithUser(context.Background(), &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})
	body, _ := json.Marshal(map[string]string{"source_repo_url": origin.URL, "module_id": "x"})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

func TestAPI_RegisterRejectsBadSignature(t *testing.T) {
	pool := newTestPool(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/manifest.json"):
			io.WriteString(w, `{"name":"x","label":"X","version":"0.1.0"}`)
		case strings.HasSuffix(r.URL.Path, ".raw.cosign"):
			io.WriteString(w, `{"fake":"bundle"}`)
		case strings.HasSuffix(r.URL.Path, ".raw"):
			w.Write([]byte("raw"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()
	api := NewAPI(db.NewModulesRepo(pool), NewDiskBlobStore(t.TempDir()), origin.Client())
	api.verifier = fakeVerifier{err: errors.New("bad signature")}
	api.allowLoopback = true
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin_badsig", "badsig@example.com")
	require.NoError(t, err)
	ctx := authkit.WithUser(context.Background(), &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})
	body, _ := json.Marshal(map[string]string{"source_repo_url": origin.URL, "module_id": "x"})
	req := httptest.NewRequest("POST", "/api/admin/modules", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	api.HandleRegister(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	rows, _ := api.repo.ListVersions(context.Background(), "x")
	require.Len(t, rows, 0) // nothing ingested on a bad signature
}

func TestAPI_HandleServeStatic(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	store := NewDiskBlobStore(t.TempDir())
	ctx := context.Background()

	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(ctx, "wk_panel", "panel@example.com")
	require.NoError(t, err)
	roversRepo := db.NewRoversRepo(pool)
	_, err = roversRepo.Create(ctx, "panel-rover", "Panel Rover", "pk", "jwt", "seed", u.ID)
	require.NoError(t, err)
	require.NoError(t, repo.RegisterModule(ctx, db.RegisterModuleInput{
		ModuleID: "umr", DisplayName: "UMR", SourceRepoURL: "https://github.com/example/umr",
		ExpectedIdentitySAN: "^test$", RegisteredBy: u.ID,
	}))
	require.NoError(t, repo.SetDesired(ctx, db.SetDesiredInput{
		RoverID: "panel-rover", ModuleID: "umr", DesiredVersion: "0.2.0", ConfigTOML: "", UpdatedBy: u.ID,
	}))

	panelBytes := []byte("/* fake panel.js */\nconsole.log('hello');")
	require.NoError(t, store.PutStatic(ctx, "umr", "0.2.0", "panel.js", panelBytes))
	meshBytes := []byte("solid mesh")
	require.NoError(t, store.PutStatic(ctx, "umr", "0.2.0", "models/arm/mesh.stl", meshBytes))

	api := NewAPI(repo, store, http.DefaultClient)

	serve := func(moduleID, p string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/admin/rovers/panel-rover/modules/"+moduleID+"/static/"+p, nil)
		req.SetPathValue("roverID", "panel-rover")
		req.SetPathValue("moduleID", moduleID)
		req.SetPathValue("path", p)
		rec := httptest.NewRecorder()
		api.HandleServeStatic(rec, req)
		return rec
	}

	// Desired version present → 200 with JS body.
	rec := serve("umr", "panel.js")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "text/javascript; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, panelBytes, rec.Body.Bytes())

	// Nested tree paths are served too.
	recMesh := serve("umr", "models/arm/mesh.stl")
	require.Equal(t, http.StatusOK, recMesh.Code, recMesh.Body.String())
	require.Equal(t, meshBytes, recMesh.Body.Bytes())

	// Module not desired for rover → 404.
	require.Equal(t, http.StatusNotFound, serve("other", "panel.js").Code)

	// Traversal attempts collapse to a cache path, never the filesystem.
	require.Equal(t, http.StatusNotFound, serve("umr", "../../../etc/passwd").Code)
	require.Equal(t, http.StatusNotFound, serve("umr", "").Code)
}

// The registry echoes each version's config schema out of its stored release
// manifest, which is the only source the enable form has: the module is not
// attached yet, so it is not publishing a schema on infra.modules.
func TestHandleList_EchoesConfigSchema(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	api := NewAPI(repo, NewDiskBlobStore(t.TempDir()), http.DefaultClient)
	ctx := context.Background()
	u, err := db.NewUsersRepo(pool).UpsertFromWorkOS(ctx, "wk_schema", "schema@example.com")
	require.NoError(t, err)

	require.NoError(t, repo.RegisterModule(ctx, db.RegisterModuleInput{
		ModuleID: "umr", DisplayName: "Connectivity", SourceRepoURL: "https://github.com/waypointos/waypoint-umr",
		ExpectedIdentitySAN: "^test$", RegisteredBy: u.ID,
	}))
	manifest := []byte(`{"name":"umr","label":"Connectivity","version":"0.5.0","config":{"fields":[
		{"key":"host","label":"Router URL","type":"url","default":"https://192.168.105.1"},
		{"key":"password","label":"Owner password","type":"password","required":true}]}}`)
	require.NoError(t, repo.IngestVersion(ctx, db.IngestVersionInput{
		ModuleID: "umr", Version: "0.5.0", RawPath: "/p", RawSha256: "s", ManifestJSON: manifest,
	}))
	// A version predating the schema must simply carry none.
	require.NoError(t, repo.IngestVersion(ctx, db.IngestVersionInput{
		ModuleID: "umr", Version: "0.4.0", RawPath: "/p", RawSha256: "s",
		ManifestJSON: []byte(`{"name":"umr","version":"0.4.0"}`),
	}))

	rr := httptest.NewRecorder()
	api.HandleList(rr, httptest.NewRequest("GET", "/api/admin/modules", nil))
	require.Equal(t, 200, rr.Code)

	var body struct {
		Modules []struct {
			ModuleID string `json:"module_id"`
			Versions []struct {
				Version      string        `json:"version"`
				ConfigFields []configField `json:"config_fields"`
			} `json:"versions"`
		} `json:"modules"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Len(t, body.Modules, 1)
	byVersion := map[string][]configField{}
	for _, v := range body.Modules[0].Versions {
		byVersion[v.Version] = v.ConfigFields
	}
	require.Len(t, byVersion["0.5.0"], 2)
	require.Equal(t, configField{
		Key: "host", Label: "Router URL", Type: "url", Default: "https://192.168.105.1",
	}, byVersion["0.5.0"][0])
	require.True(t, byVersion["0.5.0"][1].Required)
	require.Empty(t, byVersion["0.4.0"])
}

func TestHandleListDesired(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	api := NewAPI(repo, NewDiskBlobStore(t.TempDir()), http.DefaultClient)
	ctx := context.Background()

	// rover_module_state FKs on rovers.id and module_registry.module_id.
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(ctx, "wk_desired_test", "desired@example.com")
	require.NoError(t, err)
	roversRepo := db.NewRoversRepo(pool)
	_, err = roversRepo.Create(ctx, "r1", "Test Rover", "pubkey", "jwt", "seed", u.ID)
	require.NoError(t, err)
	require.NoError(t, repo.RegisterModule(ctx, db.RegisterModuleInput{
		ModuleID: "umr", DisplayName: "UMR", SourceRepoURL: "https://github.com/example/umr",
		ExpectedIdentitySAN: "^test$", RegisteredBy: u.ID,
	}))

	require.NoError(t, repo.SetDesired(ctx, db.SetDesiredInput{
		RoverID: "r1", ModuleID: "umr", DesiredVersion: "0.2.0", ConfigTOML: "x=1", UpdatedBy: u.ID,
	}))

	req := httptest.NewRequest("GET", "/api/admin/rovers/r1/modules", nil)
	req.SetPathValue("roverID", "r1")
	rr := httptest.NewRecorder()
	api.HandleListDesired(rr, req)

	require.Equal(t, 200, rr.Code)
	var body struct {
		Desired []map[string]any `json:"desired"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Len(t, body.Desired, 1)
	require.Equal(t, "umr", body.Desired[0]["module_id"])
	require.Equal(t, "0.2.0", body.Desired[0]["version"])
	require.Equal(t, "x=1", body.Desired[0]["config_toml"])
}

func TestAPI_UnpinAndDeleteModule(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin_del", "del@example.com")
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		"rover-del", "Del Rover", "pubkey-del", u.ID)
	require.NoError(t, err)

	require.NoError(t, repo.RegisterModule(context.Background(), db.RegisterModuleInput{
		ModuleID: "delmod", DisplayName: "Del Mod", SourceRepoURL: "https://github.com/x/delmod",
		ExpectedIdentitySAN: "^test$", RegisteredBy: u.ID,
	}))
	store := NewDiskBlobStore(t.TempDir())
	rawPath, err := store.Put(context.Background(), "delmod", "0.1.0", bytes.NewReader([]byte("raw")))
	require.NoError(t, err)
	require.NoError(t, repo.IngestVersion(context.Background(), db.IngestVersionInput{
		ModuleID: "delmod", Version: "0.1.0", RawPath: rawPath, RawSha256: "x", ManifestJSON: []byte(`{}`),
	}))
	require.NoError(t, repo.SetDesired(context.Background(), db.SetDesiredInput{
		RoverID: "rover-del", ModuleID: "delmod", DesiredVersion: "0.1.0", UpdatedBy: u.ID,
	}))

	api := NewAPI(repo, store, http.DefaultClient)

	// Registry delete is refused while a rover still has the module pinned.
	reqBlocked := httptest.NewRequest("DELETE", "/api/admin/modules/delmod", nil)
	reqBlocked.SetPathValue("moduleID", "delmod")
	recBlocked := httptest.NewRecorder()
	api.HandleDeleteModule(recBlocked, reqBlocked)
	require.Equal(t, http.StatusConflict, recBlocked.Code)
	require.Contains(t, recBlocked.Body.String(), "rover-del")

	// Unpin clears the rover's desired entry.
	reqUnpin := httptest.NewRequest("DELETE", "/api/admin/rovers/rover-del/modules/delmod", nil)
	reqUnpin.SetPathValue("roverID", "rover-del")
	reqUnpin.SetPathValue("moduleID", "delmod")
	recUnpin := httptest.NewRecorder()
	api.HandleUnpin(recUnpin, reqUnpin)
	require.Equal(t, http.StatusOK, recUnpin.Code, recUnpin.Body.String())
	desired, err := repo.DesiredForRover(context.Background(), "rover-del")
	require.NoError(t, err)
	require.Empty(t, desired)

	// Delete now succeeds: registry row, versions, and blob are gone.
	reqDel := httptest.NewRequest("DELETE", "/api/admin/modules/delmod", nil)
	reqDel.SetPathValue("moduleID", "delmod")
	recDel := httptest.NewRecorder()
	api.HandleDeleteModule(recDel, reqDel)
	require.Equal(t, http.StatusOK, recDel.Code, recDel.Body.String())
	_, gerr := repo.GetModule(context.Background(), "delmod")
	require.Error(t, gerr)
	versions, err := repo.ListVersions(context.Background(), "delmod")
	require.NoError(t, err)
	require.Empty(t, versions)
	_, statErr := os.Stat(rawPath)
	require.True(t, os.IsNotExist(statErr), "blob should be deleted")
}

func TestAPI_ServeStaticRepairsMissingCache(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	store := NewDiskBlobStore(t.TempDir())
	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin_panel", "panel@example.com")
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		"rover-panel", "Panel Rover", "pubkey-panel", u.ID)
	require.NoError(t, err)

	// Ingest a version with a real squashfs raw but WITHOUT caching its panel
	// (the pre-fix check-updates path did exactly this).
	rawBytes, err := os.ReadFile("testdata/umr-0.2.0.raw")
	require.NoError(t, err)
	require.NoError(t, repo.RegisterModule(context.Background(), db.RegisterModuleInput{
		ModuleID: "umr", DisplayName: "UMR", SourceRepoURL: "https://github.com/x/umr",
		ExpectedIdentitySAN: "^test$", RegisteredBy: u.ID,
	}))
	rawPath, err := store.Put(context.Background(), "umr", "0.2.0", bytes.NewReader(rawBytes))
	require.NoError(t, err)
	require.NoError(t, repo.IngestVersion(context.Background(), db.IngestVersionInput{
		ModuleID: "umr", Version: "0.2.0", RawPath: rawPath, RawSha256: "x",
		ManifestJSON: []byte(`{"name":"umr","ui":{"static":{"bundle":"/dashboard/panel.js"}}}`),
	}))
	require.NoError(t, repo.SetDesired(context.Background(), db.SetDesiredInput{
		RoverID: "rover-panel", ModuleID: "umr", DesiredVersion: "0.2.0", UpdatedBy: u.ID,
	}))

	api := NewAPI(repo, store, http.DefaultClient)
	req := httptest.NewRequest("GET", "/api/admin/rovers/rover-panel/modules/umr/static/panel.js", nil)
	req.SetPathValue("roverID", "rover-panel")
	req.SetPathValue("moduleID", "umr")
	req.SetPathValue("path", "panel.js")
	rec := httptest.NewRecorder()
	api.HandleServeStatic(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Greater(t, rec.Body.Len(), 1000)
	require.Equal(t, "text/javascript; charset=utf-8", rec.Header().Get("Content-Type"))

	// The repair also re-caches: a direct GetStatic now succeeds.
	rc, err := store.GetStatic(context.Background(), "umr", "0.2.0", "panel.js")
	require.NoError(t, err)
	rc.Close()
}

func TestAPI_HandleMintStaticToken(t *testing.T) {
	key := DeriveStaticKey([]byte("seed"))
	api := NewAPI(nil, nil, http.DefaultClient).WithStaticKey(key)

	req := httptest.NewRequest("GET", "/api/admin/rovers/r1/modules/m1/static-token", nil)
	req.SetPathValue("roverID", "r1")
	req.SetPathValue("moduleID", "m1")
	rec := httptest.NewRecorder()
	api.HandleMintStaticToken(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.True(t, VerifyStaticToken(key, "r1", "m1", out.Token, time.Now()))
	// Bound to the rover/module pair it was minted for.
	require.False(t, VerifyStaticToken(key, "r2", "m1", out.Token, time.Now()))
	require.False(t, VerifyStaticToken(key, "r1", "m2", out.Token, time.Now()))

	// No key configured → refuse to mint.
	rec500 := httptest.NewRecorder()
	NewAPI(nil, nil, http.DefaultClient).HandleMintStaticToken(rec500, req)
	require.Equal(t, http.StatusInternalServerError, rec500.Code)
}
