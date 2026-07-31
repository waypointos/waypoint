// Package nav holds the dead-reckoning pose estimate and planar geometry
// helpers for the survey mission.
package nav

import "math"

// Vec is a point in the local mission frame, meters.
type Vec struct {
	X float64
	Y float64
}

// Pose is position plus heading. Theta is radians from +X, CCW positive,
// matching the sign convention of DriveCommand.yaw_rate_radps.
type Pose struct {
	X     float64
	Y     float64
	Theta float64
}

func (p Pose) Position() Vec { return Vec{X: p.X, Y: p.Y} }

// Integrate advances the pose by one odometry sample (body forward velocity
// and yaw rate over dt seconds). Euler integration is plenty at 50 Hz.
func (p *Pose) Integrate(vx, yawRate, dt float64) {
	p.X += vx * math.Cos(p.Theta) * dt
	p.Y += vx * math.Sin(p.Theta) * dt
	p.Theta = WrapAngle(p.Theta + yawRate*dt)
}

// WrapAngle normalizes an angle to (-pi, pi].
func WrapAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

func Dist(a, b Vec) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}

// BearingTo returns the heading-relative bearing from the pose to a target,
// wrapped. Positive means the target is to the left (CCW).
func BearingTo(p Pose, t Vec) float64 {
	return WrapAngle(math.Atan2(t.Y-p.Y, t.X-p.X) - p.Theta)
}
