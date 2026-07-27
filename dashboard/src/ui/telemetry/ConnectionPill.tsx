// dashboard/src/ui/telemetry/ConnectionPill.tsx
//
// Renders one of: Direct · 4 ms | Via proxy · 32 ms | Offline.
import { Wifi, WifiOff, Cloud } from 'lucide-react';
import { Chip } from '../primitives/Chip';

type Connection =
  | { kind: 'direct'; rttMs: number }
  | { kind: 'proxy';  rttMs: number }
  | { kind: 'offline' };

type Props = {
  conn: Connection;
};

export function ConnectionPill({ conn }: Props) {
  switch (conn.kind) {
    case 'direct':
      return (
        <Chip icon={<Wifi size={12} />}>
          Direct · <span style={{ fontFamily: 'var(--font-mono)' }}>{conn.rttMs} ms</span>
        </Chip>
      );
    case 'proxy':
      return (
        <Chip icon={<Cloud size={12} />}>
          Proxy · <span style={{ fontFamily: 'var(--font-mono)' }}>{conn.rttMs} ms</span>
        </Chip>
      );
    case 'offline':
      return (
        <Chip tone="fault" icon={<WifiOff size={12} />}>
          Offline
        </Chip>
      );
  }
}
