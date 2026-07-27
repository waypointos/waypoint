// dashboard/src/ui/data/LinearGauge.tsx
import styles from './LinearGauge.module.css';

type Props = {
  /** 0..1. Clamped. */
  value: number;
  /** Below this, fill is amber. */
  warnAt?: number;
  /** Below this, fill is red. */
  faultAt?: number;
  width?: number | string;
};

export function LinearGauge({ value, warnAt = 0.25, faultAt = 0.1, width = '100%' }: Props) {
  const clamped = Math.max(0, Math.min(1, value));
  const cls =
    clamped <= faultAt ? styles.fault :
    clamped <= warnAt  ? styles.warn  :
    '';
  return (
    <div className={styles.bar} style={{ width }}>
      <i className={cls} style={{ width: `${clamped * 100}%` }} />
    </div>
  );
}
