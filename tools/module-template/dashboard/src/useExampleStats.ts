import { useEffect, useState } from 'react';
import { useSubscribe } from './bridge';
import { ExampleStats } from './proto/example_pb';

// useExampleStats subscribes to the module's stats subject and decodes each
// protobuf frame. Decode errors are ignored so a malformed frame never crashes
// the panel.
export function useExampleStats(roverId: string): { stats: ExampleStats | null } {
  const subscribe = useSubscribe();
  const [stats, setStats] = useState<ExampleStats | null>(null);
  useEffect(() => {
    const off = subscribe(`waypoint.${roverId}.module.example.stats`, (bytes) => {
      try {
        setStats(ExampleStats.fromBinary(bytes));
      } catch {
        /* ignore decode errors */
      }
    });
    return off;
  }, [subscribe, roverId]);
  return { stats };
}
