package clockanchor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestBuildAnchorMath(t *testing.T) {
	wall := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	s := &waypointv1.Stamp{
		T:      timestamppb.New(wall),
		MonoNs: uint64(90 * time.Minute), // booted 90 minutes ago
		BootId: "boot-abc",
	}
	a := build(s, true)
	assert.Equal(t, "boot-abc", a.BootId)
	assert.True(t, a.NtpSynced)
	require.NotNil(t, a.WallAtMonoZero)
	assert.Equal(t, wall.Add(-90*time.Minute), a.WallAtMonoZero.AsTime())
	assert.Same(t, s, a.Stamp)
}

func TestShouldPublish(t *testing.T) {
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	// State change publishes regardless of elapsed time.
	assert.True(t, shouldPublish(true, false, base, base.Add(time.Second)))
	// Unchanged state republishes only after the re-announce interval.
	assert.False(t, shouldPublish(true, true, base, base.Add(30*time.Second)))
	assert.True(t, shouldPublish(true, true, base, base.Add(61*time.Second)))
}
