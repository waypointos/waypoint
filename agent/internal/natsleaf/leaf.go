// Package natsleaf attaches a leaf-node remote to the agent's embedded nats-server,
// using rover-scoped JWT credentials to dial the Waypoint proxy hub.
package natsleaf

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// AttachToOptions mutates opts to add a remote leaf pointing at the proxy.
// proxyWSS must be a ws:// or wss:// URL; the user JWT + user seed are
// written to a tmp creds file because nats-server only accepts
// Credentials by path.
func AttachToOptions(opts *server.Options, proxyWSS, userJWT, userSeed string) error {
	u, err := url.Parse(proxyWSS)
	if err != nil {
		return fmt.Errorf("parse proxy url: %w", err)
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return fmt.Errorf("proxy url must be ws:// or wss://, got %s", u.Scheme)
	}
	// nats-server's options processing (server/opts.go) unconditionally
	// appends DEFAULT_LEAFNODE_PORT (7422) to any remote URL without an
	// explicit port, regardless of scheme. For ws/wss endpoints fronted
	// by an HTTPS ingress that only exposes 443, this rewrites
	// wss://host/leaf to wss://host:7422/leaf and the dial targets the
	// wrong port. Pin the scheme's standard port up front so the opts
	// auto-default sees a port already set and keeps it.
	if u.Port() == "" {
		switch u.Scheme {
		case "wss":
			u.Host = u.Host + ":443"
		case "ws":
			u.Host = u.Host + ":80"
		}
	}
	credsPath, err := writeCreds(userJWT, userSeed)
	if err != nil {
		return err
	}
	remote := &server.RemoteLeafOpts{
		URLs:        []*url.URL{u},
		Credentials: credsPath,
	}
	opts.LeafNode.Remotes = append(opts.LeafNode.Remotes, remote)
	opts.LeafNode.ReconnectInterval = 5 * time.Second
	return nil
}

func writeCreds(jwt, seed string) (string, error) {
	tmp, err := os.CreateTemp("", "waypoint-leaf-*.creds")
	if err != nil {
		return "", err
	}
	body := fmt.Sprintf(`-----BEGIN NATS USER JWT-----
%s
------END NATS USER JWT------

************************* IMPORTANT *************************
NKEY Seed printed below can be used to sign and prove identity.
NKEYs are sensitive and should be treated as secrets.

-----BEGIN USER NKEY SEED-----
%s
------END USER NKEY SEED------
*************************************************************
`, jwt, seed)
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}
