package nav

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntegrateStraight(t *testing.T) {
	p := Pose{}
	for i := 0; i < 100; i++ {
		p.Integrate(0.5, 0, 0.02) // 2 s at 0.5 m/s
	}
	assert.InDelta(t, 1.0, p.X, 1e-9)
	assert.InDelta(t, 0.0, p.Y, 1e-9)
	assert.InDelta(t, 0.0, p.Theta, 1e-9)
}

func TestIntegrateHeadingRotatesPath(t *testing.T) {
	p := Pose{Theta: math.Pi / 2}
	p.Integrate(1.0, 0, 1.0)
	assert.InDelta(t, 0.0, p.X, 1e-9)
	assert.InDelta(t, 1.0, p.Y, 1e-9)
}

func TestIntegrateYawWraps(t *testing.T) {
	p := Pose{Theta: math.Pi - 0.1}
	p.Integrate(0, 0.2, 1.0)
	// Crossed +pi, must wrap into (-pi, pi].
	assert.InDelta(t, -math.Pi+0.1, p.Theta, 1e-9)
}

func TestWrapAngle(t *testing.T) {
	assert.InDelta(t, 0.0, WrapAngle(2*math.Pi), 1e-9)
	assert.InDelta(t, math.Pi, WrapAngle(math.Pi), 1e-9)
	assert.InDelta(t, math.Pi, WrapAngle(-math.Pi), 1e-9)
	assert.InDelta(t, -math.Pi/2, WrapAngle(3*math.Pi/2), 1e-9)
}

func TestBearingTo(t *testing.T) {
	p := Pose{X: 0, Y: 0, Theta: 0}
	assert.InDelta(t, 0.0, BearingTo(p, Vec{X: 5, Y: 0}), 1e-9)
	assert.InDelta(t, math.Pi/2, BearingTo(p, Vec{X: 0, Y: 5}), 1e-9)
	// Facing +Y, a target on +X is 90 degrees to the right.
	p.Theta = math.Pi / 2
	assert.InDelta(t, -math.Pi/2, BearingTo(p, Vec{X: 5, Y: 0}), 1e-9)
}

func TestDist(t *testing.T) {
	assert.InDelta(t, 5.0, Dist(Vec{}, Vec{X: 3, Y: 4}), 1e-9)
}
