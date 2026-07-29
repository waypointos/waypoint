// dashboard/src/lib/episode/plotGeometry.ts
//
// Pure math for canvas plot lanes: extents, polyline segments broken at N/A,
// and time/pixel mapping shared by lanes and the scrubber.
import type { Sample } from './decoders';

export type Extent = { lo: number; hi: number };

export function seriesExtent(samples: Sample[]): Extent | null {
  const vals = samples.map((s) => s.value).filter((v): v is number => v !== null);
  if (vals.length === 0) return null;
  let lo = Math.min(...vals);
  let hi = Math.max(...vals);
  // A flat series has no span to normalize against; pad it so it draws mid-lane.
  if (lo === hi) { lo -= 1; hi += 1; }
  return { lo, hi };
}

export function timeToX(tNs: bigint, startNs: bigint, endNs: bigint, w: number): number {
  const span = Number(endNs - startNs) || 1;
  return (Number(tNs - startNs) / span) * w;
}

export function xToTime(x: number, startNs: bigint, endNs: bigint, w: number): bigint {
  const span = Number(endNs - startNs);
  const frac = w > 0 ? Math.min(Math.max(x / w, 0), 1) : 0;
  return startNs + BigInt(Math.round(frac * span));
}

export function toPolyline(
  samples: Sample[], startNs: bigint, endNs: bigint,
  w: number, h: number, ext: Extent,
): Array<Array<[number, number]>> {
  const segs: Array<Array<[number, number]>> = [];
  let cur: Array<[number, number]> = [];
  for (const s of samples) {
    if (s.value === null) {
      if (cur.length) segs.push(cur);
      cur = [];
      continue;
    }
    const x = timeToX(s.tNs, startNs, endNs, w);
    const y = h - ((s.value - ext.lo) / (ext.hi - ext.lo)) * h;
    cur.push([x, y]);
  }
  if (cur.length) segs.push(cur);
  return segs;
}

/** Latest sample at or before the cursor; null when it is N/A or absent. */
export function valueAtCursor(samples: Sample[], tNs: bigint): number | null {
  let latest: Sample | null = null;
  for (const s of samples) {
    if (s.tNs <= tNs && (latest === null || s.tNs > latest.tNs)) latest = s;
  }
  return latest?.value ?? null;
}
