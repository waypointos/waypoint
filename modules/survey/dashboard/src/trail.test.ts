import { appendCapped, shouldAppend, TRAIL_CAP } from './trail';

describe('shouldAppend', () => {
  it('always appends the first point', () => {
    expect(shouldAppend(null, { x: 0, y: 0, theta: 0 })).toBe(true);
  });

  it('skips sub-threshold motion', () => {
    const last = { x: 0, y: 0, theta: 0 };
    expect(shouldAppend(last, { x: 0.14, y: 0, theta: 0 })).toBe(false);
    expect(shouldAppend(last, { x: 0, y: 0, theta: (9 * Math.PI) / 180 })).toBe(false);
  });

  it('appends on distance or turn threshold', () => {
    const last = { x: 0, y: 0, theta: 0 };
    expect(shouldAppend(last, { x: 0.15, y: 0, theta: 0 })).toBe(true);
    expect(shouldAppend(last, { x: 0, y: 0, theta: (10 * Math.PI) / 180 })).toBe(true);
    // Turn deltas wrap: pi-epsilon to -pi+epsilon is a small turn.
    expect(shouldAppend({ x: 0, y: 0, theta: Math.PI - 0.01 }, { x: 0, y: 0, theta: -Math.PI + 0.01 })).toBe(
      false,
    );
  });
});

describe('appendCapped', () => {
  it('drops the oldest point at the cap', () => {
    let pts: [number, number][] = [];
    for (let i = 0; i < TRAIL_CAP + 5; i++) pts = appendCapped(pts, [i, 0]);
    expect(pts).toHaveLength(TRAIL_CAP);
    expect(pts[0][0]).toBe(5);
    expect(pts[pts.length - 1][0]).toBe(TRAIL_CAP + 4);
  });
});
