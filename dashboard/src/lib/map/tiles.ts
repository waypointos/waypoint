// Basemap tile wiring for the location map.
//
// The dashboard reads tiles from a SAME-ORIGIN XYZ endpoint, so the renderer
// never depends on an external service at runtime:
//   - served by the agent: a read-through cache (offline-capable)
//   - served by the proxy:  a session-gated pmtiles file on a volume
//   - dev: Vite proxies /basemap to a local agent or `pmtiles serve`

// Absolute (origin-prefixed) URLs. MapLibre fetches tiles/glyphs from a Web
// Worker; Safari resolves a relative URL against the worker's blob: base and
// rejects it ("URL is not valid or contains user credentials"). The origin
// prefix keeps these same-origin (still the proxy/agent) but always valid.
const ORIGIN = typeof window !== 'undefined' ? window.location.origin : '';

/** Same-origin XYZ vector tiles (Protomaps v4 schema, MVT). */
export const TILE_URL = `${ORIGIN}/basemap/{z}/{x}/{y}.mvt`;

/** Same-origin vendored glyphs — no runtime CDN. */
export const GLYPHS = `${ORIGIN}/mapfonts/{fontstack}/{range}.pbf`;

/** Tileset zoom bounds (world overview + city detail; see the seed script). */
export const MINZOOM = 0;
export const MAXZOOM = 15;

/** Default view when a rover has no GPS fix. */
export const PARIS = { lng: 2.3522, lat: 48.8566 } as const;
