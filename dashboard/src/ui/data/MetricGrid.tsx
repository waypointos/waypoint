// dashboard/src/ui/data/MetricGrid.tsx
import React from 'react';

type Props = {
  columns?: number;
  gap?: number;
  children?: React.ReactNode;
};

export function MetricGrid({ columns = 2, gap = 10, children }: Props) {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${columns}, 1fr)`,
        gap,
      }}
    >
      {children}
    </div>
  );
}
