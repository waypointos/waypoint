package ice

import (
	"os"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestResolve_NoEnvReturnsHostOnly(t *testing.T) {
	os.Unsetenv("CLOUDFLARE_TURN_KEY")
	srvs := Resolve()
	if len(srvs) != 0 {
		t.Fatalf("expected no servers, got %d", len(srvs))
	}
}

func TestResolve_CloudflareTURN(t *testing.T) {
	t.Setenv("CLOUDFLARE_TURN_KEY", "tk_abc:tu_def")
	srvs := Resolve()
	if len(srvs) == 0 {
		t.Fatal("expected at least one turn server")
	}
	if !strings.Contains(srvs[0].URLs[0], "turn:") && !strings.Contains(srvs[0].URLs[0], "turns:") {
		t.Fatalf("not a turn URL: %v", srvs[0].URLs)
	}
}

func TestResolve_MergesEnvAndProxyProvided(t *testing.T) {
	t.Setenv("CLOUDFLARE_TURN_KEY", "tk_env:tu_env")
	SetProxyProvided([]webrtc.ICEServer{{URLs: []string{"turn:proxy.example:3478"}, Username: "u", Credential: "c"}})
	t.Cleanup(func() { SetProxyProvided(nil) })
	srvs := Resolve()
	if len(srvs) != 2 {
		t.Fatalf("got %d servers, want env+proxy=2", len(srvs))
	}
	if srvs[0].Username != "tk_env" {
		t.Fatalf("first server should be env-provided, got %+v", srvs[0])
	}
	if srvs[1].URLs[0] != "turn:proxy.example:3478" {
		t.Fatalf("second server should be proxy-provided, got %+v", srvs[1])
	}
}
