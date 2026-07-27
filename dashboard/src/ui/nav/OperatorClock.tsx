// dashboard/src/ui/nav/OperatorClock.tsx
//
// Top-bar mission clock: HH:MM:SS LOCAL. Updates once per second.
import { useEffect, useState } from 'react';

const pad = (n: number) => String(n).padStart(2, '0');
export function format(d: Date) {
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())} LOCAL`;
}

export function OperatorClock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  return (
    <span style={{
      color: 'var(--color-fg-3)',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.04em',
      padding: '0 8px',
    }}>
      {format(now)}
    </span>
  );
}
