// dashboard/src/ui/nav/TopBar.tsx
import React from 'react';
import styles from './TopBar.module.css';

type Props = {
  left?: React.ReactNode;
  right?: React.ReactNode;
};

export function TopBar({ left, right }: Props) {
  return (
    <header className={styles.top}>
      <div className={styles.left}>{left}</div>
      <div className={styles.right}>{right}</div>
    </header>
  );
}
