// GPS-to-local-frame conversion for the committee's waypoint list. Mirrors
// tools/survey-mission/waypoints_to_local.py: equirectangular projection, x = East,
// y = North, meters, origin at the lowest-seq waypoint.
export const EARTH_R = 6371008.8;

export type GpsWaypoint = { seq: number; lat: number; lon: number };
export type LocalWaypoint = { seq: number; x: number; y: number; leg: number };

const DEG = Math.PI / 180;

// parseTargets accepts one waypoint per line, "seq, lat, lon[, alt]",
// tolerating parentheses, trailing degree signs, blank and # lines.
export function parseTargets(text: string): GpsWaypoint[] {
  const out: GpsWaypoint[] = [];
  for (const raw of text.split('\n')) {
    let line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    line = line.replace(/^\(+/, '').replace(/\)+$/, '');
    const parts = line.split(',').map((p) => p.trim().replace(/[°º]+$/, ''));
    if (parts.length < 3) throw new Error(`expected "seq, lat, lon[, alt]": ${raw.trim()}`);
    const seq = Number(parts[0]);
    const lat = Number(parts[1]);
    const lon = Number(parts[2]);
    if (!Number.isFinite(seq) || !Number.isFinite(lat) || !Number.isFinite(lon)) {
      throw new Error(`not numeric: ${raw.trim()}`);
    }
    out.push({ seq, lat, lon });
  }
  return out.sort((a, b) => a.seq - b.seq);
}

// toLocal projects sorted waypoints to the local frame. leg is the distance
// from the previous point, the first one measured from the frame origin.
export function toLocal(wps: GpsWaypoint[], origin?: { lat: number; lon: number }): LocalWaypoint[] {
  if (wps.length === 0) return [];
  const o = origin ?? { lat: wps[0].lat, lon: wps[0].lon };
  let prev = { x: 0, y: 0 };
  return wps.map((w) => {
    const x = (w.lon - o.lon) * DEG * EARTH_R * Math.cos(o.lat * DEG);
    const y = (w.lat - o.lat) * DEG * EARTH_R;
    const leg = Math.hypot(x - prev.x, y - prev.y);
    prev = { x, y };
    return { seq: w.seq, x, y, leg };
  });
}
