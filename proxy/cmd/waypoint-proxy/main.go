package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	alertspkg "github.com/waypointos/waypoint/proxy/internal/alerts"
	"github.com/waypointos/waypoint/proxy/internal/api"
	auditpkg "github.com/waypointos/waypoint/proxy/internal/audit"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/basemap"
	"github.com/waypointos/waypoint/proxy/internal/cameraproxy"
	"github.com/waypointos/waypoint/proxy/internal/cmdrelay"
	"github.com/waypointos/waypoint/proxy/internal/config"
	wpcrypto "github.com/waypointos/waypoint/proxy/internal/crypto"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/gateway"
	"github.com/waypointos/waypoint/proxy/internal/heartbeat"
	imagesapi "github.com/waypointos/waypoint/proxy/internal/images"
	"github.com/waypointos/waypoint/proxy/internal/landing"
	"github.com/waypointos/waypoint/proxy/internal/modules"
	"github.com/waypointos/waypoint/proxy/internal/natshub"
	"github.com/waypointos/waypoint/proxy/internal/operator"
	"github.com/waypointos/waypoint/proxy/internal/router"
	"github.com/waypointos/waypoint/proxy/internal/roverconn"
	"github.com/waypointos/waypoint/proxy/internal/roverrpc"
	"github.com/waypointos/waypoint/proxy/web"
	"gocloud.dev/blob"
)

// restoreRoverAccounts re-hydrates the per-process state that nats-server
// (MemAccResolver) and operator (in-memory keypair cache) both lose on
// restart. Iterates every rover row, restores its account keypair into
// the operator, and re-registers its account JWT with the hub.
//
// Rows missing credentials (rows that predate migration 0007) are logged
// and skipped — those rovers must be re-enrolled to come back online.
// A hard failure on any individual rover stops startup: it almost
// certainly indicates DB corruption or a botched migration, and silently
// half-restoring would mask the bigger problem.
func restoreRoverAccounts(ctx context.Context, rovers *db.RoversRepo, op *operator.Operator, hub *natshub.Hub) error {
	rows, err := rovers.List(ctx)
	if err != nil {
		return fmt.Errorf("list rovers: %w", err)
	}
	var restored, skipped int
	for _, rv := range rows {
		if rv.AccountJWT == nil || rv.AccountSeed == nil {
			log.Printf("rover %q has no persisted account credentials (pre-migration row); skipping — re-enroll to recover", rv.ID)
			skipped++
			continue
		}
		// Re-mint the account JWT from the seed using current code so any
		// claim shape (e.g. Exports) added since this rover was originally
		// enrolled is applied on the next restart without operator action.
		allowed := []string{"waypoint." + rv.ID + ".>"}
		freshJWT, _, err := op.RestoreAccountJWT(rv.ID, []byte(*rv.AccountSeed), allowed)
		if err != nil {
			return fmt.Errorf("restore account jwt for rover %q: %w", rv.ID, err)
		}
		if err := hub.RegisterAccount(freshJWT); err != nil {
			return fmt.Errorf("register account jwt for rover %q: %w", rv.ID, err)
		}
		restored++
	}
	log.Printf("restored %d rover account(s); skipped %d", restored, skipped)
	// Re-mint the sessions account JWT so its Imports cover every rover
	// just restored. This is what lets the proxy's internal subscribers
	// (heartbeat / audit / alerts) actually receive rover publishes.
	if err := api.RefreshSessionsAccount(ctx, rovers, op, hub); err != nil {
		return fmt.Errorf("refresh sessions account: %w", err)
	}
	return nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	op, err := operator.LoadFromSeed(cfg.OperatorNKeySeed)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	users := db.NewUsersRepo(pool)
	rovers := db.NewRoversRepo(pool)
	access := db.NewAccessRepo(pool)
	images := db.NewImagesRepo(pool)
	auditRepo := db.NewAuditRepo(pool)
	alertsRepo := db.NewAlertsRepo(pool)

	hub, err := natshub.Start(ctx, natshub.Config{
		Operator:    op,
		ClientPort:  -1,
		LeafWSPort:  7422,
		HTTPMonPort: -1,
		// Production: keep nats-server logging on so leaf-node connect /
		// disconnect / auth-rejection lines reach Railway logs. Tests
		// leave Logs zero so output stays quiet.
		Logs: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer hub.Shutdown()

	// Register a placeholder sessions account JWT (no imports yet) so the
	// account exists in the resolver before restoreRoverAccounts re-mints
	// it with the current rover-import list. The proxy's internal NATS
	// client (next block) needs the sessions account registered to
	// authenticate. RefreshSessionsAccount inside restoreRoverAccounts
	// overwrites this with the full import-bearing JWT.
	sessAcctJWT, err := op.MintSessionAccountJWT([]string{"waypoint.>"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := hub.RegisterAccount(sessAcctJWT); err != nil {
		log.Fatal(err)
	}

	// Restore every previously-enrolled rover's NATS account credentials
	// into both the operator keypair cache and the hub's account resolver,
	// then refresh the sessions account so its Imports list covers every
	// rover (without it, the proxy's internal subscribers can't see rover
	// publishes — cf. migration 0007 + operator.RoverImport).
	if err := restoreRoverAccounts(ctx, rovers, op, hub); err != nil {
		log.Fatal(err)
	}

	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		log.Fatal(err)
	}
	nc, err := nats.Connect(hub.Server.ClientURL(),
		nats.UserJWTAndSeed(internalJWT, string(internalSeed)),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Per-rover connections in each rover's OWN account. Proxy→rover RPC
	// (camera_offer) and publishes (modules.desired) go through these so they
	// are intra-account and the rover's leaf carries them both ways.
	roverConns := roverconn.NewPool(hub.Server.ClientURL(), func(roverID string) (string, string, error) {
		rv, err := rovers.Get(ctx, roverID)
		if err != nil {
			return "", "", err
		}
		j, s, err := op.MintRoverInternalUserJWT(rv.AccountPubKey, roverID)
		if err != nil {
			return "", "", err
		}
		return j, string(s), nil
	})
	defer roverConns.Close()

	// Forward dashboard cmd.* (driving) from the sessions account onto each
	// rover's own-account connection. Only control/admin sessions may publish
	// cmd.* (enforced by their session JWT), so the relay trusts what it sees.
	if err := cmdrelay.Start(nc, roverConns); err != nil {
		log.Fatalf("cmd relay: %v", err)
	}

	if _, err := heartbeat.Start(nc, rovers); err != nil {
		log.Fatal(err)
	}

	if err := auditpkg.Start(ctx, nc, auditRepo); err != nil {
		log.Fatal(err)
	}

	if err := alertspkg.StartMirror(ctx, nc, alertsRepo); err != nil {
		log.Fatal(err)
	}

	session := authkit.NewSessionCodec(cfg.WorkOSCookiePassword, cfg.CookieSecure)
	stateCodec := authkit.NewStateCodec(cfg.WorkOSCookiePassword, cfg.CookieSecure)
	workos := authkit.New(cfg.WorkOSAPIKey, cfg.WorkOSClientID, cfg.WorkOSRedirectURI)
	authDeps := &authkit.Deps{
		WorkOS:        workos,
		Inviter:       workos,
		Users:         users,
		Session:       session,
		StateCodec:    stateCodec,
		Access:        access,
		Rovers:        rovers,
		Operator:      op,
		PublicNATSURL: "/ws/nats",
		CookieSecure:  cfg.CookieSecure,
	}

	landingH := landing.New()

	rt := router.New(session, users)

	// Public — explicitly safe to expose anonymously.
	rt.Public("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	rt.Public("GET /favicon.svg", http.FileServer(http.FS(web.DistFS())))
	// Vendored map glyph fonts (static, public). Explicit route so they're served
	// from the dist tree instead of falling through to the SPA shell.
	rt.Public("GET /mapfonts/", http.FileServer(http.FS(web.DistFS())))
	rt.Public("GET /auth/login", http.HandlerFunc(authDeps.Login))
	rt.Public("GET /auth/callback", http.HandlerFunc(authDeps.Callback))
	rt.Public("GET /landing/static/", http.StripPrefix("/landing/static/", landingH.Static()))
	// Basemap tiles for the dashboard map, served same-origin from a self-hosted
	// S3 bucket (or local dir in dev). Gated so the endpoint isn't an open tile
	// proxy — a signed-in dashboard carries the session cookie. A bad/absent
	// source disables tiles rather than crashing the whole proxy.
	basemapSrv, err := basemap.New(cfg.BasemapSource)
	if err != nil {
		log.Printf("basemap: serving disabled (%v)", err)
	}
	if basemapSrv != nil {
		rt.RequireUser("GET /basemap/", basemapSrv)
	}

	// Rover→proxy RPC dispatcher: subscribes per-rover on the rover-account
	// connection so request/reply stays intra-account. The sessions-account
	// Stream import the telemetry firehose uses delivers the request, but a
	// leaf-backed publisher can't receive the reply on that import (same dead
	// end documented on roverconn.Pool). rpc.basemap_tile is registered only
	// when the bucket opened so a misconfigured volume doesn't take ice_servers
	// down with it.
	rpcHandlers := []roverrpc.Handler{cameraproxy.IceServersHandler{}}
	if basemapSrv != nil {
		rpcHandlers = append(rpcHandlers, basemap.RPCHandler{Srv: basemapSrv})
	}
	roverRPC := roverrpc.New(roverConns, rpcHandlers...)
	if err := roverRPC.EnsureAll(ctx, roverIDLister{rovers: rovers}); err != nil {
		log.Fatalf("rover rpc ensure all: %v", err)
	}
	// /leaf reverse-proxies WebSocket upgrades from on-rover agents to
	// the embedded nats-server's local WS port. The trailing slash on
	// the route pattern is load-bearing: nats-server's leaf-over-WS
	// protocol appends a "/leafnode" suffix, so the actual dial path is
	// /leaf/leafnode — and Go 1.22 ServeMux's "/leaf/" pattern matches
	// the subtree where "/leaf" alone would only match exact. StripPrefix
	// then chops the public mount path so nats-server sees /leafnode.
	// Auth happens at the NATS layer (operator-signed JWT + user seed),
	// not via HTTP session.
	rt.Public("GET /leaf/", http.StripPrefix("/leaf", hub.LeafHandler()))

	// Auth helpers — already require a session; use JSON 401 semantics.
	rt.RequireUser("POST /auth/logout", http.HandlerFunc(authDeps.Logout))
	rt.RequireUser("GET /api/me", http.HandlerFunc(authDeps.Me))
	rt.RequireUser("POST /api/auth/nats-cred", http.HandlerFunc(authDeps.NatsCred))
	rt.RequireUser("GET /api/users", authkit.RequireAdmin(http.HandlerFunc(authDeps.ListUsers)))
	rt.RequireUser("POST /api/users/invite", authkit.RequireAdmin(http.HandlerFunc(authDeps.InviteUser)))

	// PUBLIC_ORIGIN is the HTTP origin used for browser-facing URLs
	// (e.g. https://proxy.example.com). The leaf endpoint is reached as a
	// WebSocket on the same host, so the agent's identity.toml needs
	// the ws(s) scheme — NATS's leaf-node dialer rejects http(s) URLs
	// with "proxy url must be ws:// or wss://".
	leafURL := os.Getenv("LEAF_PUBLIC_URL")
	if leafURL == "" {
		u, err := url.Parse(cfg.PublicOrigin)
		if err != nil {
			log.Fatalf("invalid PUBLIC_ORIGIN %q: %v", cfg.PublicOrigin, err)
		}
		switch u.Scheme {
		case "https":
			u.Scheme = "wss"
		case "http":
			u.Scheme = "ws"
		}
		u.Path = "/leaf"
		leafURL = u.String()
	}
	api.RegisterRoutes(rt, api.Deps{
		Users:      users,
		Rovers:     rovers,
		Access:     access,
		Images:     images,
		Audit:      auditRepo,
		Alerts:     alertsRepo,
		Operator:   op,
		Hub:        hub,
		Session:    session,
		LeafURL:    leafURL,
		OperatorKP: op.SigningKey(),
		NC:         nc,
		RoverConns: roverConns,
		RoverRPC:   roverRPC,
	}, authkit.RequireAdmin)

	// Module registry API. Storage dir is overridable so containers / dev can
	// point at a writable location without baking the prod path.
	// Preference order: explicit disk dir (dev) → the Railway-connected S3
	// bucket (shared with basemap; module keys live under modules/) → default
	// disk path. Disk is ephemeral on Railway, so prod should have the bucket:
	// container replacement wipes disk blobs while Postgres keeps the version
	// rows, wedging every module fetch until versions are re-ingested.
	var moduleBlobs modules.BlobStore
	switch {
	case os.Getenv("WAYPOINT_PROXY_MODULE_STORAGE_DIR") != "":
		moduleBlobs = modules.NewDiskBlobStore(os.Getenv("WAYPOINT_PROXY_MODULE_STORAGE_DIR"))
	case os.Getenv("AWS_S3_BUCKET_NAME") != "":
		bkt, err := blob.OpenBucket(context.Background(), "s3://"+os.Getenv("AWS_S3_BUCKET_NAME"))
		if err != nil {
			// Fail loud: silently falling back to ephemeral disk would lose
			// every artifact registered before the next deploy.
			log.Fatalf("module blob bucket: %v", err)
		}
		defer bkt.Close()
		moduleBlobs = modules.NewBucketBlobStore(bkt)
	default:
		moduleBlobs = modules.NewDiskBlobStore("/var/lib/waypoint-proxy/modules")
	}
	modulesRepo := db.NewModulesRepo(pool)
	modulesAPI := modules.NewAPI(modulesRepo, moduleBlobs, http.DefaultClient)
	// baseRoverURL is the prefix the agent uses to fetch module .raw blobs
	// from the proxy. The host comes from PUBLIC_ORIGIN (browser-facing
	// origin); blob endpoints live under /api/rovers/<id>/modules/...
	baseRoverURL := cfg.PublicOrigin + "/api/rovers"
	blobKey := modules.DeriveBlobKey(cfg.OperatorNKeySeed)
	staticKey := modules.DeriveStaticKey(cfg.OperatorNKeySeed)
	pusher := modules.NewDesiredPusher(modulesRepo, roverConns, baseRoverURL, blobKey)
	modulesAPI = modulesAPI.WithPusher(pusher).WithBlobKey(blobKey).WithStaticKey(staticKey)
	gh := modules.NewGitHub(nil)
	var imagesAPI *imagesapi.API
	if raw := os.Getenv("MODULE_TOKEN_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			log.Fatalf("MODULE_TOKEN_KEY: %v", err)
		}
		box, err := wpcrypto.NewAESGCM(key)
		if err != nil {
			log.Fatalf("MODULE_TOKEN_KEY: %v", err)
		}
		modulesAPI.SetSecrets(gh, box)
		imagesAPI = imagesapi.NewAPI(db.NewReleasesRepo(pool), rovers, gh, box)
	} else {
		modulesAPI.SetSecrets(gh, nil)
		imagesAPI = imagesapi.NewAPI(db.NewReleasesRepo(pool), rovers, gh, nil)
	}
	rt.RequireUser("POST /api/admin/modules", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleRegister)))
	rt.RequireUser("GET /api/admin/modules", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleList)))
	rt.RequireUser("GET /api/admin/rovers/{roverID}/modules", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleListDesired)))
	rt.RequireUser("POST /api/admin/rovers/{roverID}/modules/{moduleID}", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleSetDesired)))
	rt.RequireUser("DELETE /api/admin/rovers/{roverID}/modules/{moduleID}", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleUnpin)))
	rt.RequireUser("DELETE /api/admin/modules/{moduleID}", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleDeleteModule)))
	// Raw blob fetch — pattern matches the URL the DesiredPusher emits to
	// rovers (<PublicOrigin>/api/rovers/<roverID>/modules/<moduleID>/<version>.raw?sig=…).
	// Public at the router: rovers hold no session; the ?sig= HMAC minted by
	// the pusher is the auth gate (verified in HandleServeRaw).
	rt.Public("GET /api/rovers/{roverID}/modules/{moduleID}/{version}", http.HandlerFunc(modulesAPI.HandleServeRaw))
	// Static assets accept a minted ?st= token OR an admin session: dynamic
	// import() drops the session cookie from module-script requests, so the
	// dashboard imports module scripts with a token from static-token; the
	// bundle's own relative asset fetches ride the session like any admin call.
	staticH := http.HandlerFunc(modulesAPI.HandleServeStatic)
	staticSessionH := authkit.RequireUser(session, users, authkit.RequireAdmin(staticH))
	rt.Public("GET /api/admin/rovers/{roverID}/modules/{moduleID}/static/{path...}",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if modules.VerifyStaticToken(staticKey, r.PathValue("roverID"), r.PathValue("moduleID"), r.URL.Query().Get("st"), time.Now()) {
				staticH.ServeHTTP(w, r)
				return
			}
			staticSessionH.ServeHTTP(w, r)
		}))
	rt.RequireUser("GET /api/admin/rovers/{roverID}/modules/{moduleID}/static-token", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleMintStaticToken)))
	rt.RequireUser("GET /api/admin/marketplace", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleMarketplace)))
	rt.RequireUser("GET /api/admin/modules/{moduleID}/readme", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleReadme)))
	rt.RequireUser("POST /api/admin/modules/{moduleID}/check-updates", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleCheckUpdates)))
	rt.RequireUser("POST /api/admin/modules/{moduleID}/deploy", authkit.RequireAdmin(http.HandlerFunc(modulesAPI.HandleDeploy)))
	rt.RequireUser("POST /api/admin/image-sources", authkit.RequireAdmin(http.HandlerFunc(imagesAPI.HandleRegisterSource)))
	rt.RequireUser("POST /api/admin/image-sources/check-updates", authkit.RequireAdmin(http.HandlerFunc(imagesAPI.HandleCheckUpdates)))
	rt.RequireUser("GET /api/admin/releases", authkit.RequireAdmin(http.HandlerFunc(imagesAPI.HandleReleasesFleet)))

	gw := gateway.New(gateway.Deps{
		Access:   access,
		Rovers:   rovers,
		Operator: op,
		NATSURL:  hub.Server.ClientURL(),
	})
	rt.RequireUser("GET /ws/nats", http.HandlerFunc(gw.Handle))

	cameraproxy.New(rt, cameraproxy.Deps{
		Conns: roverConns,
		Wrap: func(next http.Handler) http.Handler {
			return cameraproxy.RequireCameraAccess(access, next)
		},
	})

	// SPA assets — real files, FileServer 404s on miss as expected.
	rt.RequireUserHTML("GET /assets/", web.Handler())

	// Root — branches on session. Authed deep links (/admin, /rover/:id, ...)
	// are not real files, so the authed handler must serve the SPA shell and
	// let React Router claim the path client-side.
	rt.SPAGate(web.ShellHandler(), http.HandlerFunc(landingH.Index))

	// Wrap top-level handler with TrustForwarded for X-Forwarded-Proto/Host.
	handler := router.TrustForwarded(rt.Handler())

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: handler}

	go func() {
		log.Printf("waypoint-proxy listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
	srv.Shutdown(ctx)
}

// roverIDLister adapts db.RoversRepo to roverrpc.IDLister so the dispatcher
// package stays free of any DB dependency.
type roverIDLister struct{ rovers *db.RoversRepo }

func (r roverIDLister) IDs(ctx context.Context) ([]string, error) {
	rows, err := r.rovers.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, rv := range rows {
		ids = append(ids, rv.ID)
	}
	return ids, nil
}
