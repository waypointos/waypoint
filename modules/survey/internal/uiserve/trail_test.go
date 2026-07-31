package uiserve

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

func TestTrailDecimatesByDistance(t *testing.T) {
	tr := &Trail{}
	// 1 cm steps: only every 15th sample survives, plus the first.
	for i := 0; i <= 300; i++ {
		tr.Append(nav.Pose{X: float64(i) * 0.01})
	}
	pts := tr.Points()
	// ~3 m / 0.15 m spacing: two orders fewer points than samples, every
	// kept pair at least the threshold apart.
	assert.InDelta(t, 21, len(pts), 1)
	assert.Equal(t, [2]float64{0, 0}, pts[0])
	assert.Greater(t, pts[len(pts)-1][0], 2.8)
	for i := 1; i < len(pts); i++ {
		assert.GreaterOrEqual(t, pts[i][0]-pts[i-1][0], trailMinDistM-1e-9)
	}
}

func TestTrailAppendsOnTurnOnly(t *testing.T) {
	tr := &Trail{}
	tr.Append(nav.Pose{})
	tr.Append(nav.Pose{Theta: 5 * math.Pi / 180}) // below threshold
	assert.Len(t, tr.Points(), 1)
	tr.Append(nav.Pose{Theta: 12 * math.Pi / 180})
	assert.Len(t, tr.Points(), 2)
}

func TestTrailCapSlides(t *testing.T) {
	tr := &Trail{}
	for i := 0; i < trailCap+10; i++ {
		tr.Append(nav.Pose{X: float64(i)})
	}
	pts := tr.Points()
	assert.Len(t, pts, trailCap)
	// The oldest points fell off; the newest survived.
	assert.Equal(t, float64(10), pts[0][0])
	assert.Equal(t, float64(trailCap+9), pts[len(pts)-1][0])
}

func TestTrailReset(t *testing.T) {
	tr := &Trail{}
	tr.Append(nav.Pose{X: 1})
	tr.Reset()
	assert.Empty(t, tr.Points())
	// Post-reset the next pose always lands.
	tr.Append(nav.Pose{X: 1.01})
	assert.Len(t, tr.Points(), 1)
}
