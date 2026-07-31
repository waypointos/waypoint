import type { MissionDoc } from './types';

// SVG mission map: planned route (dashed), believed trail (solid), numbered
// waypoint markers colored by status, and the rover pose as a heading
// arrow. World frame is x=East, y=North; screen y is flipped.
type Props = {
  doc: MissionDoc | null;
  trail: [number, number][];
  compact?: boolean;
};

const W = 640;
const H = 420;
const PAD = 30;

type Fit = { sx: (x: number) => number; sy: (y: number) => number; scale: number };

function fitBounds(doc: MissionDoc | null, trail: [number, number][]): Fit {
  const xs: number[] = [];
  const ys: number[] = [];
  const add = (x: number, y: number) => {
    xs.push(x);
    ys.push(y);
  };
  if (doc) {
    for (const [x, y] of doc.planned) add(x, y);
    for (const wp of doc.waypoints) add(wp.x, wp.y);
    add(doc.pose.x, doc.pose.y);
  }
  for (const [x, y] of trail) add(x, y);
  if (xs.length === 0) add(0, 0);
  let minX = Math.min(...xs);
  let maxX = Math.max(...xs);
  let minY = Math.min(...ys);
  let maxY = Math.max(...ys);
  // Keep a sane minimum span so a parked rover is not a point at max zoom.
  const grow = (min: number, max: number): [number, number] => {
    if (max - min < 2) {
      const c = (min + max) / 2;
      return [c - 1, c + 1];
    }
    return [min, max];
  };
  [minX, maxX] = grow(minX, maxX);
  [minY, maxY] = grow(minY, maxY);
  const scale = Math.min((W - 2 * PAD) / (maxX - minX), (H - 2 * PAD) / (maxY - minY));
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  return {
    sx: (x: number) => W / 2 + (x - cx) * scale,
    sy: (y: number) => H / 2 - (y - cy) * scale,
    scale,
  };
}

// scaleBar picks a round meter length that renders 50-150 px wide.
function scaleBar(scale: number): { meters: number; px: number } {
  const nice = [0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500];
  for (const m of nice) {
    if (m * scale >= 50) return { meters: m, px: m * scale };
  }
  const m = nice[nice.length - 1];
  return { meters: m, px: m * scale };
}

function poly(pts: [number, number][], fit: Fit): string {
  return pts.map(([x, y]) => `${fit.sx(x).toFixed(1)},${fit.sy(y).toFixed(1)}`).join(' ');
}

export function MissionMap({ doc, trail, compact }: Props) {
  const fit = fitBounds(doc, trail);
  const bar = scaleBar(fit.scale);
  const empty = !doc && trail.length === 0;

  return (
    <div className="sv-map">
      <svg
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="xMidYMid meet"
        className="sv-map-svg"
        role="img"
        aria-label="Mission map"
      >
        {doc && doc.planned.length > 1 && (
          <polyline className="sv-map-planned" points={poly(doc.planned, fit)} />
        )}
        {trail.length > 1 && <polyline className="sv-map-trail" points={poly(trail, fit)} />}
        {doc?.waypoints.map((wp) => (
          <g
            key={wp.i}
            className={`sv-wp sv-wp-${wp.status}`}
            transform={`translate(${fit.sx(wp.x).toFixed(1)} ${fit.sy(wp.y).toFixed(1)})`}
          >
            <circle r={10} />
            <text className="sv-wp-num" y={3.5}>
              {wp.i + 1}
            </text>
            {wp.status === 'detected' && (
              <text className="sv-wp-id" y={-14}>
                id {wp.detected_id}
              </text>
            )}
          </g>
        ))}
        {doc && (
          <g
            className="sv-pose"
            transform={`translate(${fit.sx(doc.pose.x).toFixed(1)} ${fit
              .sy(doc.pose.y)
              .toFixed(1)}) rotate(${((-doc.pose.theta * 180) / Math.PI).toFixed(1)})`}
          >
            <polygon points="11,0 -7,6 -3,0 -7,-6" />
          </g>
        )}
        {empty && (
          <text className="sv-map-empty" x={W / 2} y={H / 2}>
            N/A: no mission doc from module
          </text>
        )}
        <g className="sv-scale" transform={`translate(${PAD} ${H - 16})`}>
          <line x1={0} y1={0} x2={bar.px} y2={0} />
          <line x1={0} y1={-4} x2={0} y2={4} />
          <line x1={bar.px} y1={-4} x2={bar.px} y2={4} />
          <text x={bar.px + 8} y={3.5}>
            {bar.meters} m
          </text>
        </g>
      </svg>
      {!compact && (
        <div className="sv-map-footer">
          <span className="sv-legend-item">
            <span className="sv-swatch sv-swatch-planned" /> planned
          </span>
          <span className="sv-legend-item">
            <span className="sv-swatch sv-swatch-trail" /> trail
          </span>
          <span className="sv-legend-item">
            <span className="sv-dot sv-dot-pending" /> pending
          </span>
          <span className="sv-legend-item">
            <span className="sv-dot sv-dot-reached" /> reached
          </span>
          <span className="sv-legend-item">
            <span className="sv-dot sv-dot-detected" /> detected
          </span>
        </div>
      )}
    </div>
  );
}
