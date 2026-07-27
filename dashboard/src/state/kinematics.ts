// dashboard/src/state/kinematics.ts
//
// Skid-steer (differential drive) helpers. The dashboard does not own
// kinematics; geometry comes from the runtime platform (state/platform.ts)
// so the display tracks the connected rover's descriptor.

// Stick caps: what we publish in DriveCommand when the joystick is at full
// deflection. These are operator tunables, not platform geometry.
export const MAX_VX_MPS    = 0.6;
export const MAX_YAW_RADPS = 1.0;

export type Geometry = { wheelRadiusM: number; trackWidthM: number };

export type StickInput = { vx: number; yaw: number };

/** Result in rad/s, signed (positive = forward rotation for that side). */
export type WheelTargets = {
  left:  number;
  right: number;
};

/**
 * Compute per-side target angular velocity for a skid-steer rover.
 * Formula: v_left = vx - wz * (track / 2),  v_right = vx + wz * (track / 2),
 * omega = v / r. Inputs are physical units (m/s, rad/s); use
 * {@link stickToPhysical} first if you have normalized stick values.
 */
export function wheelTargets(g: Geometry, vxMps: number, yawRadps: number): WheelTargets {
  const halfTrack = g.trackWidthM / 2;
  const vL = vxMps - yawRadps * halfTrack;
  const vR = vxMps + yawRadps * halfTrack;
  return {
    left:  vL / g.wheelRadiusM,
    right: vR / g.wheelRadiusM,
  };
}

/**
 * Convert a normalized stick value (-1..1) into the physical command we publish.
 * Stick yaw is positive-right (operator convention); DriveCommand.yaw_rate_radps
 * is positive-counter-clockwise (left), so the sign flips here.
 */
export function stickToPhysical(s: StickInput): { vxMps: number; yawRadps: number } {
  return {
    vxMps:    s.vx  * MAX_VX_MPS,
    yawRadps: -s.yaw * MAX_YAW_RADPS,
  };
}
