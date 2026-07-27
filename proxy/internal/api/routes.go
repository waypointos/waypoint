package api

import (
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/natshub"
	"github.com/waypointos/waypoint/proxy/internal/operator"
	"github.com/waypointos/waypoint/proxy/internal/router"
)

type Deps struct {
	Users    *db.UsersRepo
	Rovers   *db.RoversRepo
	Access   *db.AccessRepo
	Images   *db.ImagesRepo
	Audit    *db.AuditRepo
	Alerts   *db.AlertsRepo
	Operator *operator.Operator
	Hub      *natshub.Hub
	Session  *authkit.SessionCodec
	LeafURL  string
	// OperatorKP is the operator nkeys keypair, used for signing magic links.
	OperatorKP nkeys.KeyPair
	// NC is the proxy's local NATS connection, used for rover RPC fan-out.
	NC *nats.Conn
	// RoverConns is the rover-account connection pool. Conn yields a connection
	// in a rover's own account (proxy→rover RPC routes intra-account,
	// leaf-carried). Remove evicts a deleted rover's cached connection.
	// Optional (nil in tests).
	RoverConns roverConns
	// RoverRPC owns per-rover subscriptions for rover→proxy RPCs (basemap_tile,
	// ice_servers). Enroll calls Ensure on the new rover, Delete calls Remove
	// before the underlying connection is evicted. Optional (nil in tests).
	RoverRPC roverRPCRegistry
}

// roverConnRemover narrows roverconn.Pool to what the delete handler needs.
type roverConnRemover interface {
	Remove(roverID string)
}

// roverRPCRegistry narrows roverrpc.Dispatcher to the enroll/delete surface so
// tests can stub it without spinning up a real NATS hub.
type roverRPCRegistry interface {
	Ensure(roverID string) error
	Remove(roverID string)
}

// roverConns is the rover-account connection pool: Conn yields a connection in
// a rover's own account (proxy→rover RPC routes intra-account, leaf-carried),
// Remove evicts a deleted rover's cached connection.
type roverConns interface {
	Conn(roverID string) (*nats.Conn, error)
	Remove(roverID string)
}

// RegisterRoutes wires the JSON API into the typed router. Every route is
// gated by RequireUser (session required). Admin-only routes additionally
// compose the admin gate inside.
func RegisterRoutes(r *router.Router, d Deps, admin func(http.Handler) http.Handler) {
	r.RequireUser("GET /api/rovers", newListHandler(d.Rovers, d.Access))
	r.RequireUser("POST /api/rovers/enroll", admin(newEnrollHandler(d.Rovers, d.Operator, d.Hub, d.LeafURL, d.RoverRPC)))
	r.RequireUser("DELETE /api/rovers/{id}", admin(newDeleteRoverHandler(d.Rovers, d.Operator, d.Hub, d.NC, d.RoverConns, d.RoverRPC)))
	r.RequireUser("POST /api/rovers/{id}/access", admin(newGrantHandler(d.Access)))
	r.RequireUser("DELETE /api/rovers/{id}/access/{userId}", admin(newRevokeHandler(d.Access)))
	r.RequireUser("POST /api/rovers/{id}/magic-link", newMagicLinkHandler(d.Access, d.Rovers, d.OperatorKP))
	r.RequireUser("POST /api/rovers/{id}/apply-image", admin(newApplyImageHandler(d.Images, func(roverID string) (natsRequester, error) {
		if d.RoverConns == nil {
			return nil, fmt.Errorf("rover connections unavailable")
		}
		conn, err := d.RoverConns.Conn(roverID)
		return conn, err
	})))
	r.RequireUser("GET /api/audit", newAuditListHandler(d.Audit, d.Access))
	r.RequireUser("GET /api/alerts", newAlertsListHandler(d.Alerts, d.Access))
	r.RequireUser("POST /api/alerts/{id}/acknowledge", newAlertsAckHandler(d.Alerts, d.Access, d.NC))
	r.RequireUser("POST /api/alerts/{id}/resolve", newAlertsResolveHandler(d.Alerts, d.Access, d.NC))
}
