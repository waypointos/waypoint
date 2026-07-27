// dashboard/src/ui/primitives/Panel.tsx
import React from 'react';
import styles from './Panel.module.css';

type Props = {
  /** Panel heading, rendered with `[ ... ]` bracket prefix in mono caps. */
  title?: string;
  /** Right-aligned note next to the heading (e.g., "live · 50 hz"). */
  note?: string;
  /** Optional control rendered at the far right of the header (after the note). */
  action?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
};

export function Panel({ title, note, action, children, className, style }: Props) {
  return (
    <section
      className={[styles.panel, className].filter(Boolean).join(' ')}
      style={style}
    >
      {title ? (
        <h3 className={styles.h}>
          <span>
            <span className={styles.hPrefix}>[</span>
            {title}
            <span className={styles.hPrefix}>]</span>
          </span>
          {note || action ? (
            <span className={styles.right}>
              {note ? <span className={styles.n}>{note}</span> : null}
              {action}
            </span>
          ) : null}
        </h3>
      ) : null}
      {children}
    </section>
  );
}
