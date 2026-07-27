// dashboard/src/ui/data/Metric.tsx
//
// Single metric tile. Renders muted "N/A" + reason hint when value is null/undefined.
import styles from './Metric.module.css';

type Tone = 'default' | 'key' | 'warn';

type Props = {
  label: string;
  /** Number, string, or null/undefined → N/A. */
  value?: number | string | null;
  unit?: string;
  /** Required when value is null/undefined. Shown beneath the muted N/A. */
  naReason?: string;
  /** Visual emphasis: 'key' (primary), 'warn' (caution amber). */
  tone?: Tone;
  /** Formatter for numeric values; defaults to (v) => String(v). */
  format?: (v: number) => string;
};

const defaultFormat = (v: number) => (Number.isInteger(v) ? String(v) : v.toFixed(2));

export function Metric({
  label, value, unit, naReason, tone = 'default', format = defaultFormat,
}: Props) {
  const cls = [styles.m, tone === 'key' && styles.k, tone === 'warn' && styles.w]
    .filter(Boolean).join(' ');
  const isNA = value === null || value === undefined;
  return (
    <div className={cls}>
      <div className={styles.l}>{label}</div>
      {isNA ? (
        <>
          <div className={styles.na}>N/A</div>
          {naReason ? <div className={styles.nahint}>{naReason}</div> : null}
        </>
      ) : (
        <div className={styles.v}>
          {typeof value === 'number' ? format(value) : value}
          {unit ? <span className={styles.u}>{unit}</span> : null}
        </div>
      )}
    </div>
  );
}
