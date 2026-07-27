// Hand-written MapLibre style for the Waypoint console basemap.
//
// Not an off-the-shelf theme — each vector source-layer (Protomaps v4 schema)
// is painted from the design tokens so the map reads as part of the dashboard.
// WebGL needs literal colors, so token values are referenced via `color`;
// a few map-only shades (water, road tiers, boundary) extend the ramp and are
// centralised here. Keep in sync with tokens.css.
import type { StyleSpecification } from 'maplibre-gl';
import { color } from '@/ui/tokens';
import { GLYPHS, MAXZOOM, MINZOOM, TILE_URL } from './tiles';

const M = {
  land:      color.bg,        // #0A0A0B
  landAlt:   color.chrome,    // #0F0F11
  water:     '#0B0F15',       // map-only: faint cool tint
  roadMinor: color.border,    // #26262C
  roadMajor: '#3A3A42',       // map-only: one step above border
  building:  color.surface2,  // #1B1B1F
  boundary:  '#2A2A30',       // map-only
  label:     color.fg3,       // #828289
  labelHalo: color.bg,
};

/**
 * Build the basemap style.
 * @param pitched when true, buildings extrude in 3D; when false they're flat fills (top-down).
 */
export function basemapStyle({ pitched }: { pitched: boolean }): StyleSpecification {
  const buildings = pitched
    ? {
        id: 'buildings', type: 'fill-extrusion' as const, source: 'pm', 'source-layer': 'buildings',
        minzoom: 13,
        paint: {
          'fill-extrusion-color': M.building,
          'fill-extrusion-height': ['coalesce', ['get', 'height'], 12],
          'fill-extrusion-base': 0,
          'fill-extrusion-opacity': 1,
        },
      }
    : {
        id: 'buildings', type: 'fill' as const, source: 'pm', 'source-layer': 'buildings',
        minzoom: 14,
        paint: { 'fill-color': M.building, 'fill-outline-color': M.roadMinor },
      };

  return {
    version: 8,
    glyphs: GLYPHS,
    sources: {
      pm: {
        type: 'vector',
        tiles: [TILE_URL],
        minzoom: MINZOOM,
        maxzoom: MAXZOOM,
        attribution: '© OpenStreetMap · Protomaps',
      },
    },
    layers: [
      { id: 'bg', type: 'background', paint: { 'background-color': M.land } },
      { id: 'earth', type: 'fill', source: 'pm', 'source-layer': 'earth', paint: { 'fill-color': M.land } },
      { id: 'landuse', type: 'fill', source: 'pm', 'source-layer': 'landuse', paint: { 'fill-color': M.landAlt } },
      { id: 'water', type: 'fill', source: 'pm', 'source-layer': 'water', paint: { 'fill-color': M.water } },
      {
        id: 'roads-minor', type: 'line', source: 'pm', 'source-layer': 'roads',
        paint: { 'line-color': M.roadMinor, 'line-width': ['interpolate', ['linear'], ['zoom'], 11, 0.4, 17, 2.4] },
      },
      {
        id: 'roads-major', type: 'line', source: 'pm', 'source-layer': 'roads',
        filter: ['any', ['==', 'kind', 'highway'], ['==', 'kind', 'major_road'], ['==', 'kind', 'medium_road']],
        paint: { 'line-color': M.roadMajor, 'line-width': ['interpolate', ['linear'], ['zoom'], 11, 1, 17, 6] },
      },
      buildings,
      {
        id: 'boundaries', type: 'line', source: 'pm', 'source-layer': 'boundaries',
        paint: { 'line-color': M.boundary, 'line-dasharray': [2, 2], 'line-width': 0.8 },
      },
      {
        id: 'places', type: 'symbol', source: 'pm', 'source-layer': 'places',
        layout: {
          'text-field': ['get', 'name'], 'text-font': ['Noto Sans Regular'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 10, 9, 16, 13],
        },
        paint: { 'text-color': M.label, 'text-halo-color': M.labelHalo, 'text-halo-width': 1.4 },
      },
    ],
  } as StyleSpecification;
}
