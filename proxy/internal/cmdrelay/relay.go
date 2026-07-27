// Package cmdrelay forwards browser-published cmd.* frames, the
// browser-initiated control RPCs (rpc.set_mode/rpc.estop/rpc.recover), and
// interactive-module command frames (module.*.command) from the sessions
// account onto each rover's own-account connection, so they reach the
// leaf-backed rover. A cross-account stream export would form a NATS import
// cycle with the sessions account's rover-wildcard import; relaying through the
// rover-account connection keeps the two directions cleanly separated and
// preserves the full waypoint.<id>.> firehose import.
package cmdrelay

import (
	"strings"

	"github.com/nats-io/nats.go"
)

// relayedHeader marks a frame this relay has already republished. The rover
// account exports waypoint.<id>.> and the sessions account imports it verbatim,
// so a frame republished into the rover account re-enters the sessions account
// on the same subject and re-triggers this relay's own subscription. Tagging the
// republish and dropping already-tagged frames breaks that loop.
const relayedHeader = "Waypoint-Relayed"

// roverConns yields a NATS connection in a rover's own account.
type roverConns interface {
	Conn(roverID string) (*nats.Conn, error)
}

// Start subscribes to waypoint.*.cmd.> and the three browser-initiated control
// RPC subjects (rpc.set_mode/rpc.estop/rpc.recover) on sub (the proxy's
// sessions-account connection) and republishes each frame on the rover's
// own-account connection from conns. Authorisation is enforced upstream: only a
// control/admin session JWT may publish those subjects into the sessions account.
//
// Frames for an unreachable rover are dropped, not logged: cmd is a 50 Hz
// stream (the next frame arrives in ~20 ms) and the operator already sees the
// rover's offline state via the telemetry panes — per-frame error logging would
// only spam the log during an outage the operator can already see.
func Start(sub *nats.Conn, conns roverConns) error {
	handle := func(m *nats.Msg) {
		if m.Header.Get(relayedHeader) != "" {
			return // our own republish, re-imported via the firehose — drop it
		}
		roverID := roverIDFromSubject(m.Subject)
		if roverID == "" {
			return
		}
		nc, err := conns.Conn(roverID)
		if err != nil {
			return
		}
		out := nats.NewMsg(m.Subject)
		out.Data = m.Data
		out.Header.Set(relayedHeader, "1")
		_ = nc.PublishMsg(out)
	}

	// cmd.* is the 50 Hz drive stream; the three rpc.* leaves are the
	// browser-initiated control RPCs (mode switch / E-stop / clear). All are
	// fire-and-forget — the result returns on event.mode, not a reply — so an
	// allow-list of subjects is all the relay needs. rpc.camera_offer /
	// rpc.apply_image stay proxy-initiated and are deliberately not listed.
	subjects := []string{
		"waypoint.*.cmd.>",
		"waypoint.*.rpc.set_mode",
		"waypoint.*.rpc.estop",
		"waypoint.*.rpc.recover",
		"waypoint.*.module.*.command",
	}
	for _, s := range subjects {
		if _, err := sub.Subscribe(s, handle); err != nil {
			return err
		}
	}
	return nil
}

// roverIDFromSubject extracts <id> from waypoint.<id>.<...>; "" if malformed.
func roverIDFromSubject(subject string) string {
	parts := strings.SplitN(subject, ".", 3)
	if len(parts) < 3 || parts[0] != "waypoint" {
		return ""
	}
	return parts[1]
}
