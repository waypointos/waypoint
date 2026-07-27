// dashboard/src/state/speed.ts
//
// Dashboard-side speed scaling for drive commands. The core keeps its own
// velocity limits; this only scales the normalized stick (-1..1) before it is
// handed to useDriveCmd. published = stick * presetFactor * cap.
export type SpeedPreset = 'slow' | 'cruise' | 'turbo';

export const PRESET_FACTOR: Record<SpeedPreset, number> = {
  slow: 0.35,
  cruise: 0.65,
  turbo: 1.0,
};

export function applySpeedScale(
  stick: { vx: number; yaw: number },
  preset: SpeedPreset,
  cap: number,
): { vx: number; yaw: number } {
  const f = PRESET_FACTOR[preset] * Math.max(0, Math.min(1, cap));
  return { vx: stick.vx * f, yaw: stick.yaw * f };
}
