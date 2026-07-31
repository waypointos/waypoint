// Client-side trail extension mirrors the module's decimation (uiserve
// Trail) so points appended from live pose updates line up with what a
// later trail.get would return.
export const TRAIL_MIN_DIST_M = 0.15;
export const TRAIL_MIN_TURN_RAD = (10 * Math.PI) / 180;
export const TRAIL_CAP = 4000;

export type PosePoint = { x: number; y: number; theta: number };

export function wrapAngle(a: number): number {
  while (a > Math.PI) a -= 2 * Math.PI;
  while (a <= -Math.PI) a += 2 * Math.PI;
  return a;
}

export function shouldAppend(last: PosePoint | null, next: PosePoint): boolean {
  if (!last) return true;
  const moved = Math.hypot(next.x - last.x, next.y - last.y) >= TRAIL_MIN_DIST_M;
  const turned = Math.abs(wrapAngle(next.theta - last.theta)) >= TRAIL_MIN_TURN_RAD;
  return moved || turned;
}

export function appendCapped(pts: [number, number][], p: [number, number]): [number, number][] {
  const out = pts.length >= TRAIL_CAP ? pts.slice(pts.length - TRAIL_CAP + 1) : pts.slice();
  out.push(p);
  return out;
}
