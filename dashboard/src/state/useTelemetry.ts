// dashboard/src/state/useTelemetry.ts
//
// Subscribe to a NATS subject, decode protobuf bytes with the caller's decoder,
// keep the latest message in React state.
import { useEffect, useState } from 'react';
import { getBus } from './nats';

export function useTelemetry<T>(subject: string, decode: (b: Uint8Array) => T): T | null {
  const [val, setVal] = useState<T | null>(null);
  useEffect(() => {
    const off = getBus().subscribe(subject, (bytes) => {
      try {
        setVal(decode(bytes));
      } catch {
        // ignore decode errors
      }
    });
    return off;
  }, [subject, decode]);
  return val;
}
