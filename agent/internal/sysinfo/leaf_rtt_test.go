package sysinfo

import (
	"errors"
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

type fakeLeafz struct {
	leafz *natsserver.Leafz
	err   error
}

func (f fakeLeafz) Leafz(_ *natsserver.LeafzOptions) (*natsserver.Leafz, error) {
	return f.leafz, f.err
}

func TestLeafRTTReader_NoLeafReturnsFalse(t *testing.T) {
	r := NewLeafRTTReader(fakeLeafz{leafz: &natsserver.Leafz{Leafs: nil}})
	if _, ok := r.LatestMs(); ok {
		t.Fatal("expected (_, false) when no leaf is connected")
	}
}

func TestLeafRTTReader_LeafzErrorReturnsFalse(t *testing.T) {
	r := NewLeafRTTReader(fakeLeafz{err: errors.New("boom")})
	if _, ok := r.LatestMs(); ok {
		t.Fatal("expected (_, false) when Leafz returns an error")
	}
}

func TestLeafRTTReader_EmptyRTTStringReturnsFalse(t *testing.T) {
	// nats-server omits the RTT string until the first PING/PONG completes
	// for a leaf; we must not treat that as 0 ms.
	r := NewLeafRTTReader(fakeLeafz{leafz: &natsserver.Leafz{
		Leafs: []*natsserver.LeafInfo{{RTT: ""}},
	}})
	if _, ok := r.LatestMs(); ok {
		t.Fatal("expected (_, false) when leaf RTT hasn't been measured yet")
	}
}

func TestLeafRTTReader_ParsesDurationStringsToMilliseconds(t *testing.T) {
	cases := []struct {
		name   string
		rttStr string
		wantMs float64
	}{
		{"sub-ms (microseconds)", "523µs", 0.523},
		{"low-single-digit ms", "4.2ms", 4.2},
		{"two-digit ms", "32ms", 32},
		{"three-digit ms with fractional", "150.5ms", 150.5},
		{"second", "1s", 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewLeafRTTReader(fakeLeafz{leafz: &natsserver.Leafz{
				Leafs: []*natsserver.LeafInfo{{RTT: tc.rttStr}},
			}})
			got, ok := r.LatestMs()
			if !ok {
				t.Fatalf("LatestMs returned (_, false) for valid RTT string %q", tc.rttStr)
			}
			// Float compare with a small epsilon — durations don't round-trip
			// to bit-exact decimal milliseconds.
			if diff := got - tc.wantMs; diff > 0.01 || diff < -0.01 {
				t.Fatalf("got %v ms, want %v ms", got, tc.wantMs)
			}
		})
	}
}

func TestLeafRTTReader_UnparseableStringReturnsFalse(t *testing.T) {
	// Defensive: if nats-server ever changes its RTT formatting, we'd
	// rather report N/A than a bogus 0 ms value.
	r := NewLeafRTTReader(fakeLeafz{leafz: &natsserver.Leafz{
		Leafs: []*natsserver.LeafInfo{{RTT: "not-a-duration"}},
	}})
	if _, ok := r.LatestMs(); ok {
		t.Fatal("expected (_, false) for an unparseable RTT string")
	}
}
