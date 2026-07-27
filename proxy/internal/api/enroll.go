package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

type roverCreator interface {
	Create(ctx context.Context, id, name, accountPubkey, accountJWT, accountSeed string, enrolledBy uuid.UUID) (*db.Rover, error)
}
type roverLister interface {
	List(ctx context.Context) ([]db.Rover, error)
}
type hubRegistrar interface {
	RegisterAccount(jwt string) error
}

func newEnrollHandler(rovers interface {
	roverCreator
	roverLister
}, op *operator.Operator, hub hubRegistrar, leafURL string, rpc roverRPCRegistry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := authkit.UserFrom(r.Context())
		var body struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || body.Name == "" {
			http.Error(w, "bad request", 400)
			return
		}

		allowed := []string{"waypoint." + body.ID + ".>"}
		accountJWT, accountPK, accountSeed, err := op.MintAccountJWT(body.ID, allowed)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// nil perms: the rover user inherits the account's DefaultPermissions,
		// so future claims changes ride RestoreAccountJWT on restart with no re-enroll.
		userJWT, userSeed, err := op.MintRoverUserJWT(accountJWT, nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := hub.RegisterAccount(accountJWT); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// Persist both JWT and seed: the JWT alone would let us re-register
		// existing user JWTs after restart, but without the seed in cache
		// we couldn't mint any NEW user JWTs under this account post-restart.
		if _, err := rovers.Create(r.Context(), body.ID, body.Name, accountPK, accountJWT, string(accountSeed), u.ID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Refresh the sessions account JWT so its Imports list includes the
		// rover we just enrolled. Without this, the proxy's internal
		// subscribers (heartbeat consumer, audit, alerts) cannot see this
		// rover's publishes — NATS account isolation drops them at the
		// account boundary.
		if err := RefreshSessionsAccount(r.Context(), rovers, op, hub); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Subscribe per-rover RPC responders (basemap_tile, ice_servers) on the
		// new rover's own-account connection. A failure here doesn't fail the
		// enrollment: the rover is already created and reachable; a future
		// proxy restart's EnsureAll will retry.
		if rpc != nil {
			if err := rpc.Ensure(body.ID); err != nil {
				log.Printf("enroll: roverrpc ensure %q: %v", body.ID, err)
			}
		}

		bootstrap := randHex(16)
		toml := renderIdentityTOML(body.ID, body.Name, leafURL, op.PublicKey(), userJWT, string(userSeed), bootstrap)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"roverId":      body.ID,
			"identityToml": toml,
		})
	})
}

// RefreshSessionsAccount re-mints the sessions account JWT with one
// Stream Import per known rover and re-registers it with the hub. Called
// after every enrollment (handler above) and at startup (main.go's
// restoreRoverAccounts) so the import list stays in sync with the
// rovers table.
//
// The sessions account's signing key is deterministic from the operator
// seed, so existing session-user JWTs minted against the previous
// account claims remain valid across the re-mint.
func RefreshSessionsAccount(ctx context.Context, rovers roverLister, op *operator.Operator, hub hubRegistrar) error {
	rows, err := rovers.List(ctx)
	if err != nil {
		return fmt.Errorf("list rovers: %w", err)
	}
	imports := make([]operator.RoverImport, 0, len(rows))
	for _, rv := range rows {
		if rv.AccountPubKey == "" {
			continue
		}
		imports = append(imports, operator.RoverImport{
			RoverID:       rv.ID,
			AccountPubKey: rv.AccountPubKey,
		})
	}
	sessJWT, err := op.MintSessionAccountJWT([]string{"waypoint.>"}, imports)
	if err != nil {
		return fmt.Errorf("mint sessions account: %w", err)
	}
	if err := hub.RegisterAccount(sessJWT); err != nil {
		return fmt.Errorf("register sessions account: %w", err)
	}
	return nil
}

func renderIdentityTOML(id, name, proxyURL, operatorPK, userJWT, userSeed, bootstrap string) string {
	return fmt.Sprintf(`[rover]
id   = %q
name = %q

[proxy]
url             = %q
operator_pubkey = %q

[nats]
user_jwt    = %q
user_seed   = %q

[local]
bootstrap_admin_token = %q
`, id, name, proxyURL, operatorPK, userJWT, userSeed, bootstrap)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
