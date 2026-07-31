import { parseTargets, toLocal } from './geo';

describe('parseTargets', () => {
  it('parses committee-style lines with parens, degree signs, and alt', () => {
    const wps = parseTargets(
      '# targets\n(2, 39.9017482°, 32.7704942°, 890)\n1, 39.9017797, 32.7704813\n\n',
    );
    expect(wps).toEqual([
      { seq: 1, lat: 39.9017797, lon: 32.7704813 },
      { seq: 2, lat: 39.9017482, lon: 32.7704942 },
    ]);
  });

  it('rejects malformed lines', () => {
    expect(() => parseTargets('1, 39.9')).toThrow(/seq, lat, lon/);
    expect(() => parseTargets('a, b, c')).toThrow(/not numeric/);
  });
});

describe('toLocal', () => {
  it('matches waypoints_to_local.py on the reference point', () => {
    const pts = toLocal([{ seq: 2, lat: 39.9017482, lon: 32.7704942 }], {
      lat: 39.9017797,
      lon: 32.7704813,
    });
    expect(pts).toHaveLength(1);
    expect(pts[0].x).toBeCloseTo(1.1, 2);
    expect(pts[0].y).toBeCloseTo(-3.5, 2);
    expect(pts[0].leg).toBeCloseTo(3.67, 2);
  });

  it('defaults the origin to the lowest-seq waypoint and chains legs', () => {
    const pts = toLocal(
      parseTargets('1, 39.9017797, 32.7704813\n2, 39.9017482, 32.7704942'),
    );
    expect(pts[0].x).toBeCloseTo(0, 6);
    expect(pts[0].y).toBeCloseTo(0, 6);
    expect(pts[0].leg).toBeCloseTo(0, 6);
    expect(pts[1].leg).toBeCloseTo(3.67, 2);
  });
});
