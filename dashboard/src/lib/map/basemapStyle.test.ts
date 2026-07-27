import { describe, expect, it } from 'vitest';
import { basemapStyle } from './basemapStyle';
import { TILE_URL } from './tiles';

describe('basemapStyle', () => {
  it('is a v8 style sourcing XYZ vector tiles same-origin', () => {
    const style = basemapStyle({ pitched: false });
    expect(style.version).toBe(8);
    const pm = style.sources.pm as { type: string; tiles?: string[]; url?: string };
    expect(pm.type).toBe('vector');
    expect(pm.tiles).toEqual([TILE_URL]);
    expect(pm.url).toBeUndefined();
    // Origin-prefixed absolute URL (Safari worker rejects relative ones).
    expect(TILE_URL).toMatch(/\/basemap\/\{z\}\/\{x\}\/\{y\}\.mvt$/);
  });

  it('uses an absolute tile URL (not a bare relative path)', () => {
    expect(TILE_URL).not.toMatch(/^\/basemap/);
  });

  it('extrudes buildings only when pitched', () => {
    const flat = basemapStyle({ pitched: false }).layers.find((l) => l.id === 'buildings');
    const tilt = basemapStyle({ pitched: true }).layers.find((l) => l.id === 'buildings');
    expect(flat?.type).toBe('fill');
    expect(tilt?.type).toBe('fill-extrusion');
  });
});
