// dashboard/src/ui/primitives/Chip.tsx
import React from 'react';
import styles from './Chip.module.css';

type Tone = 'default' | 'caution' | 'fault';

type Props = {
  tone?: Tone;
  dot?: boolean;
  icon?: React.ReactNode;
  number?: string | number;
  children?: React.ReactNode;
};

export function Chip({ tone = 'default', dot, icon, number, children }: Props) {
  const cls = [styles.chip, tone !== 'default' && styles[tone]].filter(Boolean).join(' ');
  return (
    <span className={cls}>
      {dot ? <span className={styles.dot} /> : null}
      {icon ?? null}
      {number != null ? <span className={styles.num}>{number}</span> : null}
      {children}
    </span>
  );
}
