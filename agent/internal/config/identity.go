// agent/internal/config/identity.go
//
// Loads /data/waypoint/identity.toml. Layout matches the enrollment payload
// the proxy generates when the user adds a rover.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

type Identity struct {
	Rover   RoverInfo    `toml:"rover"`
	Proxy   ProxyInfo    `toml:"proxy"`
	NATS    NATSInfo     `toml:"nats"`
	Local   LocalInfo    `toml:"local"`
	Cameras []CameraSpec `toml:"cameras"`
}

type CameraSpec struct {
	Name     string `toml:"name"`
	Device   string `toml:"device"`
	Pipeline string `toml:"pipeline"` // "synthetic", "mac", "pi5", or "" (auto-detect)
}

type RoverInfo struct {
	ID   string `toml:"id"`
	Name string `toml:"name"`
}

type ProxyInfo struct {
	URL           string `toml:"url"`
	OperatorPubKey string `toml:"operator_pubkey"`
}

type NATSInfo struct {
	// UserJWT is the per-rover NATS user JWT, signed by the proxy
	// operator under the rover's account. Despite its historical
	// mislabelling as "account_jwt" in both code and TOML, this has
	// always been a user JWT — leaf-node credentials files expect a
	// user JWT, and the proxy's enroll handler mints one (see
	// proxy/internal/operator.MintRoverUserJWT).
	UserJWT  string `toml:"user_jwt"`
	UserSeed string `toml:"user_seed"`
}

type LocalInfo struct {
	BootstrapAdminToken string `toml:"bootstrap_admin_token"`
	// mDNS name to advertise on the LAN (→ <name>.local). Defaults to
	// "waypoint" when empty. Optional; LAN-only convenience.
	MDNSHostname string `toml:"mdns_hostname"`
}

// Load reads identity from a TOML file and validates required fields.
func Load(path string) (*Identity, error) {
	var id Identity
	if _, err := toml.DecodeFile(path, &id); err != nil {
		return nil, fmt.Errorf("decode identity.toml: %w", err)
	}
	return validate(&id)
}

// ParseString decodes identity from a TOML string and validates required fields.
func ParseString(s string) (*Identity, error) {
	var id Identity
	if _, err := toml.NewDecoder(strings.NewReader(s)).Decode(&id); err != nil {
		return nil, fmt.Errorf("decode identity: %w", err)
	}
	return validate(&id)
}

func validate(id *Identity) (*Identity, error) {
	if id.Rover.ID == "" {
		return nil, errors.New("identity.toml: rover.id is required")
	}
	if id.Local.BootstrapAdminToken == "" {
		return nil, errors.New("identity.toml: local.bootstrap_admin_token is required")
	}
	if id.Proxy.URL != "" {
		if id.Proxy.OperatorPubKey == "" {
			return nil, errors.New("identity.toml: proxy.operator_pubkey is required when [proxy] url is set")
		}
		if id.NATS.UserJWT == "" {
			return nil, errors.New("identity.toml: nats.user_jwt is required when [proxy] url is set")
		}
		if id.NATS.UserSeed == "" {
			return nil, errors.New("identity.toml: nats.user_seed is required when [proxy] url is set")
		}
	}
	seen := make(map[string]struct{}, len(id.Cameras))
	for i, c := range id.Cameras {
		if c.Name == "" {
			return nil, fmt.Errorf("identity.toml: cameras[%d].name is required", i)
		}
		if _, dup := seen[c.Name]; dup {
			return nil, fmt.Errorf("identity.toml: duplicate camera name %q", c.Name)
		}
		seen[c.Name] = struct{}{}
	}
	return id, nil
}
