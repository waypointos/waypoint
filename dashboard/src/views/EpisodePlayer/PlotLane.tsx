// dashboard/src/views/EpisodePlayer/PlotLane.tsx
//
// One canvas plot lane per topic: polylines broken at N/A, a cursor line,
// and a legend with per-series cursor readouts.
import { useEffect, useRef } from 'react';
import {
  seriesExtent, toPolyline, timeToX, valueAtCursor, latestAtCursor, staleAfterNs,
} from '../../lib/episode/plotGeometry';
import type { Sample } from '../../lib/episode/decoders';
import styles from './PlotLane.module.css';

// Series stroke cycle; resolved from tokens at draw time.
const STROKE_VARS = ['--color-accent', '--color-fg-2', '--color-fg-3'];

type Props = {
  title: string;
  series: Map<string, Sample[]>;
  ids: string[];
  startNs: bigint;
  endNs: bigint;
  cursorNs: bigint;
};

function naReason(samples: Sample[], cursorNs: bigint, maxGapNs: bigint | null): string {
  if (samples.length === 0) return 'no samples in episode';
  const latest = latestAtCursor(samples, cursorNs);
  if (latest === null) return 'before first sample';
  if (maxGapNs != null && cursorNs - latest.tNs > maxGapNs) return 'last sample too old';
  return 'not reported at this time';
}

export function PlotLane({ title, series, ids, startNs, endNs, cursorNs }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) return;
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, w, h);
    const tokens = getComputedStyle(canvas);
    ids.forEach((id, i) => {
      const samples = series.get(id) ?? [];
      const ext = seriesExtent(samples);
      if (!ext) return;
      ctx.strokeStyle = tokens.getPropertyValue(STROKE_VARS[i % STROKE_VARS.length]).trim();
      ctx.lineWidth = 1;
      for (const seg of toPolyline(samples, startNs, endNs, w, h, ext)) {
        ctx.beginPath();
        seg.forEach(([x, y], j) => (j === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y)));
        ctx.stroke();
      }
    });
    const cx = timeToX(cursorNs, startNs, endNs, w);
    ctx.strokeStyle = tokens.getPropertyValue('--color-accent').trim();
    ctx.beginPath();
    ctx.moveTo(cx, 0);
    ctx.lineTo(cx, h);
    ctx.stroke();
  }, [series, ids, startNs, endNs, cursorNs]);

  return (
    <section className={styles.lane}>
      <header className={styles.laneTitle}>{title}</header>
      <canvas ref={canvasRef} className={styles.canvas} />
      <ul className={styles.legend}>
        {ids.map((id) => {
          const samples = series.get(id) ?? [];
          const maxGapNs = staleAfterNs(samples);
          const v = valueAtCursor(samples, cursorNs, maxGapNs);
          return (
            <li key={id} className={styles.legendRow}>
              <span className={styles.legendId}>{id}</span>
              {v === null ? (
                <span className={styles.na}>
                  N/A
                  <span className={styles.naHint}>{naReason(samples, cursorNs, maxGapNs)}</span>
                </span>
              ) : (
                <span className={styles.legendValue}>{v.toPrecision(5)}</span>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
