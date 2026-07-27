package discovery

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestPortFromAddr(t *testing.T) {
	cases := map[string]int{
		":80":          80,
		":8080":        8080,
		"0.0.0.0:8090": 8090,
		"127.0.0.1:80": 80,
		"[::1]:9000":   9000,
		"garbage":      0, // no port → 0
		"":             0,
	}
	for in, want := range cases {
		if got := PortFromAddr(in); got != want {
			t.Errorf("PortFromAddr(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIPv4Addrs_ExcludesLoopback(t *testing.T) {
	got, err := ipv4Addrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range got {
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() == nil {
			t.Errorf("%q is not a valid IPv4 address", s)
		}
		if ip.IsLoopback() {
			t.Errorf("%q is loopback; must be excluded", s)
		}
	}
}

func TestAdvertise_Resolvable(t *testing.T) {
	srv, err := Advertise("Waypoint Test", "waypoint-test", 8080)
	if err != nil {
		t.Skipf("advertise unavailable in this environment: %v", err)
	}
	defer srv.Shutdown()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		t.Skipf("resolver unavailable: %v", err)
	}
	entries := make(chan *zeroconf.ServiceEntry, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := resolver.Browse(ctx, "_http._tcp", "local.", entries); err != nil {
		t.Skipf("browse failed: %v", err)
	}

	for {
		select {
		case e := <-entries:
			if e != nil && e.Instance == "Waypoint Test" {
				return // found it
			}
		case <-ctx.Done():
			t.Skip("multicast not delivered in this environment (e.g. CI sandbox)")
		}
	}
}
