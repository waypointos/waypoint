package natsleaf

import (
	"os"
	"strings"
	"testing"

	"github.com/nats-io/nats-server/v2/server"
)

func TestAttachToOptions_AddsRemote(t *testing.T) {
	opts := &server.Options{}
	if err := AttachToOptions(opts, "wss://proxy.example.com/leaf", "fakejwt", "fakeseed"); err != nil {
		t.Fatal(err)
	}
	if len(opts.LeafNode.Remotes) != 1 {
		t.Fatalf("got %d remotes", len(opts.LeafNode.Remotes))
	}
	r := opts.LeafNode.Remotes[0]
	if r.Credentials == "" {
		t.Fatal("Credentials path must be set")
	}
	b, err := os.ReadFile(r.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "fakejwt") || !strings.Contains(string(b), "fakeseed") {
		t.Fatal("creds file missing JWT or seed")
	}
}

func TestAttachToOptions_RejectsBadScheme(t *testing.T) {
	opts := &server.Options{}
	if err := AttachToOptions(opts, "http://x/leaf", "j", "s"); err == nil {
		t.Fatal("must reject non-ws scheme")
	}
}

// nats-server appends DEFAULT_LEAFNODE_PORT (7422) to any RemoteLeafOpts
// URL without an explicit port, regardless of scheme. For ws/wss
// endpoints fronted by an HTTPS ingress that only exposes 443, that
// rewrite makes the outbound dial target the wrong port. AttachToOptions
// must pin 443 (wss) / 80 (ws) before the URL hits LeafNode.Remotes so
// the auto-default never fires.
func TestAttachToOptions_PinsSchemeDefaultPortToBeatLeafnodeDefault(t *testing.T) {
	cases := []struct {
		name, in, wantHost string
	}{
		{"wss-no-port", "wss://proxy.example.com/leaf", "proxy.example.com:443"},
		{"ws-no-port", "ws://proxy.example.com/leaf", "proxy.example.com:80"},
		{"wss-explicit-port-preserved", "wss://proxy.example.com:9443/leaf", "proxy.example.com:9443"},
		{"ws-explicit-port-preserved", "ws://proxy.example.com:8080/leaf", "proxy.example.com:8080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := &server.Options{}
			if err := AttachToOptions(opts, tc.in, "j", "s"); err != nil {
				t.Fatal(err)
			}
			got := opts.LeafNode.Remotes[0].URLs[0].Host
			if got != tc.wantHost {
				t.Fatalf("URL host = %q, want %q", got, tc.wantHost)
			}
		})
	}
}
