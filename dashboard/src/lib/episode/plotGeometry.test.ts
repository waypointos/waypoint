import { describe, expect, it } from 'vitest';
import { seriesExtent, toPolyline, timeToX, xToTime, valueAtCursor } from './plotGeometry';
import type { Sample } from './decoders';

const S = (tNs: bigint, value: number | null): Sample => ({ tNs, value });

describe('plotGeometry', () => {
  it('computes extent and pads flat series', () => {
    expect(seriesExtent([S(0n, 5), S(1n, 5)])).toEqual({ lo: 4, hi: 6 });
    expect(seriesExtent([S(0n, null)])).toBeNull();
  });

  it('breaks polylines at N/A samples', () => {
    const segs = toPolyline(
      [S(0n, 0), S(1n, 1), S(2n, null), S(3n, 3)],
      0n, 3n, 300, 100, { lo: 0, hi: 3 },
    );
    expect(segs).toHaveLength(2);
    expect(segs[0]).toHaveLength(2);
    expect(segs[1]).toHaveLength(1);
  });

  it('maps time to x and back', () => {
    expect(timeToX(5n, 0n, 10n, 200)).toBe(100);
    expect(xToTime(100, 0n, 10n, 200)).toBe(5n);
  });

  it('reads the latest value at or before the cursor, honoring N/A', () => {
    const s = [S(0n, 1), S(10n, null), S(20n, 3)];
    expect(valueAtCursor(s, 5n)).toBe(1);
    expect(valueAtCursor(s, 15n)).toBeNull();
    expect(valueAtCursor(s, 25n)).toBe(3);
    expect(valueAtCursor(s, -1n)).toBeNull();
  });
});
