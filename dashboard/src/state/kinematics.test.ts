// Pins the stick-to-wire sign convention: stick yaw is positive-right, but
// DriveCommand.yaw_rate_radps is positive-counter-clockwise (left) per the
// protocol, so stickToPhysical must flip the sign.
import { describe, expect, it } from 'vitest';
import { type Geometry, MAX_YAW_RADPS, stickToPhysical, wheelTargets } from './kinematics';

const ROVER_GEOMETRY: Geometry = { wheelRadiusM: 0.07425, trackWidthM: 0.3 };

describe('stickToPhysical', () => {
  it('maps stick-right to a negative (clockwise) yaw rate', () => {
    const { yawRadps } = stickToPhysical({ vx: 0, yaw: 1 });
    expect(yawRadps).toBe(-MAX_YAW_RADPS);
  });

  it('maps stick-right to left wheels faster than right (a right turn)', () => {
    const { vxMps, yawRadps } = stickToPhysical({ vx: 0.5, yaw: 1 });
    const t = wheelTargets(ROVER_GEOMETRY, vxMps, yawRadps);
    expect(t.left).toBeGreaterThan(t.right);
  });

  it('keeps a centered stick at zero', () => {
    const { vxMps, yawRadps } = stickToPhysical({ vx: 0, yaw: 0 });
    expect(vxMps).toBe(0);
    // -0 from negating a centered stick is fine on the wire; assert numerically.
    expect(yawRadps).toBeCloseTo(0);
  });
});
