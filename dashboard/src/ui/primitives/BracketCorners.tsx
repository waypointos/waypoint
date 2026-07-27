// dashboard/src/ui/primitives/BracketCorners.tsx
//
// Tactical corner-bracket overlay used for selection / track affordance.
// Place inside a `position: relative` parent.
import React from 'react';
import styles from './BracketCorners.module.css';

type Props = {
  /** Render all four corners; default is just top-left + bottom-right. */
  full?: boolean;
  /** CSS color override; defaults to the accent token. */
  color?: string;
  /** Bracket arm length; default 8 px. */
  size?: number;
};

export function BracketCorners({ full, color = 'var(--color-accent)', size = 8 }: Props) {
  const style: React.CSSProperties = {
    // CSS variable consumed inside the module CSS.
    ['--color' as never]: color,
    ['width' as never]: size,
    ['height' as never]: size,
  };
  const corners: Array<'tl' | 'tr' | 'bl' | 'br'> = full
    ? ['tl', 'tr', 'bl', 'br']
    : ['tl', 'br'];
  return (
    <>
      {corners.map((c) => (
        <span
          key={c}
          data-bracket-corner={c}
          className={`${styles.corner} ${styles[c]}`}
          style={style}
        />
      ))}
    </>
  );
}
