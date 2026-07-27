// Package discovery advertises the agent's HTTP service over mDNS so the
// dashboard is reachable by name (e.g. http://waypoint.local/) on the LAN.
// mDNS is link-local multicast: it works on the home LAN but does NOT traverse
// Tailscale. The caller treats advertise failures as non-fatal.
package discovery

import (
	"fmt"
	"net"
	"strconv"

	"github.com/grandcat/zeroconf"
)

// Advertise publishes an _http._tcp service and A records for <host>.local on
// all non-loopback IPv4 interfaces. Call Shutdown() on the returned server at
// exit. Returns an error (responder not started) when no usable address exists.
func Advertise(instance, host string, port int) (*zeroconf.Server, error) {
	ips, err := ipv4Addrs()
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no non-loopback IPv4 address to advertise")
	}
	return zeroconf.RegisterProxy(
		instance, "_http._tcp", "local.", port, host, ips,
		[]string{"path=/"}, nil,
	)
}

// PortFromAddr extracts the TCP port from a listen address like ":80" or
// "0.0.0.0:8090". Returns 0 when the address has no parseable port.
func PortFromAddr(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return p
}

func ipv4Addrs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ifc := range ifaces {
		// Skip down, loopback, and non-multicast interfaces: zeroconf only
		// serves mDNS on multicast-capable links, so advertising an address
		// from one it can't back is misleading.
		if ifc.Flags&net.FlagUp == 0 ||
			ifc.Flags&net.FlagLoopback != 0 ||
			ifc.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := ipnet.IP.To4(); v4 != nil {
				out = append(out, v4.String())
			}
		}
	}
	return out, nil
}
