// dashboard/src/views/EpisodePlayer/TimelineScrubber.tsx
//
// Shared timeline: click/drag seeks, shift-drag selects a range for export.
import { useRef } from 'react';
import type React from 'react';
import { timeToX, xToTime } from '../../lib/episode/plotGeometry';
import styles from './TimelineScrubber.module.css';

type Sel = { startNs: bigint; endNs: bigint };
type Props = {
  startNs: bigint;
  endNs: bigint;
  cursorNs: bigint;
  selection: Sel | null;
  onSeek: (tNs: bigint) => void;
  onSelect: (sel: Sel | null) => void;
};

export function TimelineScrubber({ startNs, endNs, cursorNs, selection, onSeek, onSelect }: Props) {
  const trackRef = useRef<HTMLDivElement>(null);
  const dragFrom = useRef<bigint | null>(null);

  function timeAt(e: React.PointerEvent): bigint {
    const track = trackRef.current;
    if (!track) return startNs;
    const rect = track.getBoundingClientRect();
    return xToTime(e.clientX - rect.left, startNs, endNs, rect.width);
  }

  function down(e: React.PointerEvent) {
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    const t = timeAt(e);
    if (e.shiftKey) {
      dragFrom.current = t;
    } else {
      dragFrom.current = null;
      onSelect(null);
      onSeek(t);
    }
  }

  function move(e: React.PointerEvent) {
    if (e.buttons === 0) return;
    const t = timeAt(e);
    if (dragFrom.current !== null) {
      const a = dragFrom.current;
      onSelect(a <= t ? { startNs: a, endNs: t } : { startNs: t, endNs: a });
    } else {
      onSeek(t);
    }
  }

  function key(e: React.KeyboardEvent) {
    const span = endNs - startNs;
    const step = span / 100n || 1n;
    const clamp = (t: bigint) => (t < startNs ? startNs : t > endNs ? endNs : t);
    const to: Record<string, bigint | undefined> = {
      ArrowLeft: cursorNs - step,
      ArrowRight: cursorNs + step,
      PageDown: cursorNs - step * 10n,
      PageUp: cursorNs + step * 10n,
      Home: startNs,
      End: endNs,
    };
    const next = to[e.key];
    if (next === undefined) return;
    e.preventDefault();
    onSeek(clamp(next));
  }

  // Positions are ratios, so any width works as the mapping denominator.
  const w = 1000;
  const pct = (t: bigint) => `${(timeToX(t, startNs, endNs, w) / w) * 100}%`;
  const elapsedS = (t: bigint) => Number(t - startNs) / 1e9;

  return (
    <div
      ref={trackRef}
      className={styles.track}
      data-testid="scrubber"
      role="slider"
      tabIndex={0}
      aria-label="Episode timeline"
      aria-valuemin={0}
      aria-valuemax={elapsedS(endNs)}
      aria-valuenow={elapsedS(cursorNs)}
      aria-valuetext={`${elapsedS(cursorNs).toFixed(2)} s`}
      onKeyDown={key}
      onPointerDown={down}
      onPointerMove={move}
    >
      {selection && (
        <div
          className={styles.selection}
          style={{
            left: pct(selection.startNs),
            width: `calc(${pct(selection.endNs)} - ${pct(selection.startNs)})`,
          }}
        />
      )}
      <div className={styles.cursor} style={{ left: pct(cursorNs) }} />
    </div>
  );
}
