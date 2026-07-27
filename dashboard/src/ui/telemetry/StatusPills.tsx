// dashboard/src/ui/telemetry/StatusPills.tsx
//
// Compact subsystem health row (core / agent / cam / ...). Tone semantics:
// ok = fresh, warn = degraded or stale, off = absent (muted, not alarming).
import styles from './StatusPills.module.css';

export type PillTone = 'ok' | 'warn' | 'off';
export type Pill = { label: string; tone: PillTone };

export function StatusPills({ pills }: { pills: Pill[] }) {
  return (
    <div className={styles.row} data-status-pills>
      {pills.map((p) => (
        <span key={p.label} className={`${styles.pill} ${styles[p.tone]}`}>{p.label}</span>
      ))}
    </div>
  );
}
