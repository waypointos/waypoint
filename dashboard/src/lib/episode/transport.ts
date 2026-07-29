// dashboard/src/lib/episode/transport.ts
//
// Pure playback clock over MCAP log times. The view drives tick() from
// requestAnimationFrame; everything here is testable without a DOM.
export type Transport = { playing: boolean; speed: number; cursorNs: bigint };

export const SPEEDS = [0.25, 0.5, 1, 2, 4] as const;

export function newTransport(startNs: bigint): Transport {
  return { playing: false, speed: 1, cursorNs: startNs };
}

export function play(t: Transport): Transport { return { ...t, playing: true }; }
export function pause(t: Transport): Transport { return { ...t, playing: false }; }

export function setSpeed(t: Transport, speed: number): Transport {
  return { ...t, speed };
}

export function tick(t: Transport, wallDtMs: number, endNs: bigint): Transport {
  if (!t.playing) return t;
  const advanced = t.cursorNs + BigInt(Math.round(wallDtMs * t.speed * 1e6));
  if (advanced >= endNs) return { ...t, cursorNs: endNs, playing: false };
  return { ...t, cursorNs: advanced };
}

export function seek(t: Transport, tNs: bigint, startNs: bigint, endNs: bigint): Transport {
  const clamped = tNs < startNs ? startNs : tNs > endNs ? endNs : tNs;
  return { ...t, cursorNs: clamped };
}

/** Greatest key time at or before t; null when t precedes every keyframe. */
export function keyframeBefore(keyTimesNs: bigint[], tNs: bigint): bigint | null {
  let best: bigint | null = null;
  for (const k of keyTimesNs) {
    if (k <= tNs && (best === null || k > best)) best = k;
  }
  return best;
}
