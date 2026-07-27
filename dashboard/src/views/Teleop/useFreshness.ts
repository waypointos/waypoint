// dashboard/src/views/Teleop/useFreshness.ts
//
// Age of the most recent non-null sample, refreshed at 1 Hz. null until the
// first sample arrives. Drives the subsystem health pills: a publisher that
// stops mid-session reads as stale rather than freezing its last value.
import { useEffect, useRef, useState } from 'react';

export function useFreshness(value: unknown): number | null {
  const lastAt = useRef<number | null>(null);
  const [, bump] = useState(0);

  useEffect(() => {
    if (value !== null && value !== undefined) lastAt.current = Date.now();
  }, [value]);

  useEffect(() => {
    const id = setInterval(() => bump((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);

  return lastAt.current === null ? null : Date.now() - lastAt.current;
}
