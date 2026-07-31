package uiserve

import (
	"math"
	"sync"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

// Decimation thresholds; the dashboard panel mirrors them so locally
// extended points line up with what trail.get returns.
const (
	trailMinDistM   = 0.15
	trailMinTurnRad = 10 * math.Pi / 180
	trailCap        = 4000
)

// Trail keeps a decimated history of the believed pose for the map view.
type Trail struct {
	mu     sync.Mutex
	pts    [][2]float64
	last   nav.Pose
	primed bool
}

// Append records the pose if it moved or turned enough since the last kept
// point. Turn-only changes count too so in-place pivots stay visible.
func (t *Trail) Append(p nav.Pose) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.primed {
		moved := math.Hypot(p.X-t.last.X, p.Y-t.last.Y) >= trailMinDistM
		turned := math.Abs(nav.WrapAngle(p.Theta-t.last.Theta)) >= trailMinTurnRad
		if !moved && !turned {
			return
		}
	}
	if len(t.pts) >= trailCap {
		t.pts = append(t.pts[:0], t.pts[1:]...)
	}
	t.pts = append(t.pts, [2]float64{p.X, p.Y})
	t.last, t.primed = p, true
}

func (t *Trail) Points() [][2]float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([][2]float64, len(t.pts))
	copy(out, t.pts)
	return out
}

func (t *Trail) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pts, t.primed = nil, false
}
