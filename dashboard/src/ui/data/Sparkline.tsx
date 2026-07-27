// dashboard/src/ui/data/Sparkline.tsx
//
// Minimal SVG sparkline. Renders a single polyline over the supplied series,
// normalized into a fixed viewport. Missing samples are drawn as gaps so a
// flapping telemetry stream is visible. No axes, no labels, no tooltips —
// pair with a Metric or Panel to provide units and current value.
import styles from './Sparkline.module.css';

type Props = {
  /** Samples in chronological order. null = missing sample (renders as a gap). */
  values: ReadonlyArray<number | null>;
  /** Optional fixed min/max. When omitted, the series's own min/max is used. */
  min?: number;
  max?: number;
  /** SVG aspect via viewBox; defaults to 120×32 unitless. */
  width?: number;
  height?: number;
  /** Stroke color CSS var or value. Defaults to --color-accent. */
  stroke?: string;
  /** When true, renders a filled area below the line as well as the stroke. */
  filled?: boolean;
};

export function Sparkline({
  values,
  min,
  max,
  width = 120,
  height = 32,
  stroke = 'var(--color-accent)',
  filled = false,
}: Props) {
  if (values.length === 0) {
    return <svg className={styles.spark} viewBox={`0 0 ${width} ${height}`} />;
  }
  const present = values.filter((v): v is number => v != null);
  if (present.length === 0) {
    return <svg className={styles.spark} viewBox={`0 0 ${width} ${height}`} />;
  }
  const lo = min ?? Math.min(...present);
  const hi = max ?? Math.max(...present);
  const span = hi - lo || 1; // avoid div-by-zero on a flat series

  const stepX = values.length > 1 ? width / (values.length - 1) : 0;

  // Build path segments, breaking on nulls so gaps are visible.
  const segments: string[] = [];
  let current = '';
  values.forEach((v, i) => {
    if (v == null) {
      if (current) { segments.push(current); current = ''; }
      return;
    }
    const x = i * stepX;
    const y = height - ((v - lo) / span) * height;
    current += current ? ` L${x.toFixed(2)},${y.toFixed(2)}` : `M${x.toFixed(2)},${y.toFixed(2)}`;
  });
  if (current) segments.push(current);

  const linePath = segments.join(' ');

  // Filled area: anchor at bottom corners. Only renders the longest contiguous
  // segment to keep the fill from doubling back through gaps.
  let areaPath = '';
  if (filled && segments.length > 0) {
    const longest = segments.reduce((a, b) => (b.length > a.length ? b : a));
    const moveMatch = longest.match(/^M([\d.\-]+),([\d.\-]+)/);
    const lastMatch = [...longest.matchAll(/L([\d.\-]+),([\d.\-]+)/g)].pop();
    if (moveMatch && lastMatch) {
      const xStart = moveMatch[1];
      const xEnd   = lastMatch[1];
      areaPath = `${longest} L${xEnd},${height} L${xStart},${height} Z`;
    }
  }

  return (
    <svg className={styles.spark} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      {areaPath && <path d={areaPath} className={styles.fill} style={{ fill: stroke }} />}
      <path d={linePath} className={styles.line} style={{ stroke }} />
    </svg>
  );
}
