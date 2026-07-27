// dashboard/src/ui/primitives/Stack.tsx
//
// Small layout primitive. Defaults to flex-row with gap.
import React from 'react';
import styles from './Stack.module.css';

type Props = {
  direction?: 'row' | 'column';
  gap?: number;        // px
  align?: React.CSSProperties['alignItems'];
  justify?: React.CSSProperties['justifyContent'];
  wrap?: boolean;
  className?: string;
  children?: React.ReactNode;
  style?: React.CSSProperties;
};

export function Stack({
  direction = 'row',
  gap = 8,
  align,
  justify,
  wrap,
  className,
  children,
  style,
}: Props) {
  return (
    <div
      className={[styles.stack, styles[direction], className].filter(Boolean).join(' ')}
      style={{
        gap,
        alignItems: align,
        justifyContent: justify,
        flexWrap: wrap ? 'wrap' : undefined,
        ...style,
      }}
    >
      {children}
    </div>
  );
}
