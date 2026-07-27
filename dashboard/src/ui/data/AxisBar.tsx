// dashboard/src/ui/data/AxisBar.tsx
//
// Bidirectional command/measured bar around a centre zero tick. The filled
// band is the commanded value; the thin underline beneath is measured.
// The underline turns caution when measured saturates against full scale.
import styles from './AxisBar.module.css';

type Props = {
  cmd: number;
  /** null/undefined renders no measured underline (telemetry absent). */
  meas?: number | null;
  /** Full-scale magnitude; both values clamp to ±max for display. */
  max: number;
  /** Fraction of max treated as saturation. */
  satAt?: number;
  'aria-label'?: string;
};

const norm = (v: number, max: number) => Math.max(-1, Math.min(1, v / max)) * 50;

export function AxisBar({ cmd, meas, max, satAt = 0.92, 'aria-label': ariaLabel }: Props) {
  const cw = norm(cmd, max);
  const mw = meas == null ? null : norm(meas, max);
  const sat = meas != null && Math.abs(meas) > satAt * max;
  return (
    <span className={styles.bar} role="img" aria-label={ariaLabel} data-axis-bar>
      <span className={styles.zero} />
      <span
        className={styles.cmd}
        style={{ left: `${cw < 0 ? 50 + cw : 50}%`, width: `${Math.abs(cw)}%` }}
      />
      {mw != null && (
        <span
          className={sat ? `${styles.meas} ${styles.sat}` : styles.meas}
          style={{ left: `${mw < 0 ? 50 + mw : 50}%`, width: `${Math.abs(mw)}%` }}
        />
      )}
    </span>
  );
}
