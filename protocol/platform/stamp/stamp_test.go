package stamp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNowFillsWallAndMono(t *testing.T) {
	s := Now()
	require.NotNil(t, s)
	require.NotNil(t, s.T)
	assert.WithinDuration(t, time.Now(), s.T.AsTime(), 2*time.Second)
	assert.Greater(t, s.MonoNs, uint64(0))
}

func TestMonoIncreases(t *testing.T) {
	a := Now()
	time.Sleep(2 * time.Millisecond)
	b := Now()
	assert.Greater(t, b.MonoNs, a.MonoNs)
}

func TestBootIDStable(t *testing.T) {
	// "" is the N/A value on hosts without /proc (macOS dev); on Linux it is a
	// UUID. Either way it must be stable across calls.
	assert.Equal(t, BootID(), BootID())
}
