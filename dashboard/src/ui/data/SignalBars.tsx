import styles from './SignalBars.module.css';

export type SignalBarsProps = {
  value: number | null;
  max?: number;
  showLabel?: boolean;
};

export function SignalBars({ value, max = 4, showLabel = false }: SignalBarsProps) {
  if (value === null || value === undefined) {
    return <span className={styles.na}>N/A</span>;
  }
  const clamped = Math.max(0, Math.min(value, max));
  const bars = Array.from({ length: max }, (_, i) => i < clamped);
  return (
    <span className={styles.row} aria-label={`Signal: ${clamped} of ${max}`}>
      {bars.map((on, i) => (
        <span
          key={i}
          className={styles.bar}
          data-active={on}
          style={{ height: `${30 + i * 18}%` }}
        />
      ))}
      {showLabel && <span className={styles.label}>{`${clamped}/${max}`}</span>}
    </span>
  );
}
