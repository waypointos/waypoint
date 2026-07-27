import { describe, expect, it } from 'vitest';
import { clampPosition, clampSize } from './floatingWindowGeometry';

const vp = { width: 1000, height: 800 };

describe('clampPosition', () => {
  it('leaves an in-bounds window untouched', () => {
    expect(clampPosition({ x: 100, y: 50 }, { width: 300, height: 200 }, vp))
      .toEqual({ x: 100, y: 50 });
  });

  it('clamps a window past the right/bottom edge', () => {
    expect(clampPosition({ x: 900, y: 700 }, { width: 300, height: 200 }, vp))
      .toEqual({ x: 700, y: 600 });
  });

  it('clamps negative positions to zero', () => {
    expect(clampPosition({ x: -40, y: -10 }, { width: 300, height: 200 }, vp))
      .toEqual({ x: 0, y: 0 });
  });

  it('pins to origin when the window is larger than the viewport', () => {
    expect(clampPosition({ x: 50, y: 50 }, { width: 1200, height: 900 }, vp))
      .toEqual({ x: 0, y: 0 });
  });
});

describe('clampSize', () => {
  it('enforces minimum size', () => {
    expect(clampSize({ width: 50, height: 40 }, { x: 0, y: 0 }, vp, { width: 200, height: 150 }))
      .toEqual({ width: 200, height: 150 });
  });

  it('caps size to the viewport from the window origin', () => {
    expect(clampSize({ width: 9999, height: 9999 }, { x: 100, y: 100 }, vp, { width: 200, height: 150 }))
      .toEqual({ width: 900, height: 700 });
  });
});
