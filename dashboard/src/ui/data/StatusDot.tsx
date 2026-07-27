// dashboard/src/ui/data/StatusDot.tsx
import styles from './StatusDot.module.css';

type Status = 'ok' | 'warn' | 'fault' | 'off';

type Props = {
  status: Status;
  label?: string;
};

export function StatusDot({ status, label }: Props) {
  return (
    <span>
      <span className={`${styles.dot} ${styles[status]}`} aria-label={status} />
      {label ? <span>{label}</span> : null}
    </span>
  );
}
