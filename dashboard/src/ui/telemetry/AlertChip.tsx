// dashboard/src/ui/telemetry/AlertChip.tsx
//
// Small severity-coloured chip used both in lists and in the TopBar count
// badge. Maps the alert severity domain (info|warning|critical) to the Chip
// primitive's tone vocabulary (default|caution|fault).
import { Info, AlertTriangle, AlertOctagon } from 'lucide-react';
import { Chip } from '@/ui/primitives/Chip';
import type { AlertSeverity } from '@/state/useAlerts';

type Props = {
  severity: AlertSeverity;
  /** Optional count rendered before the label (e.g. TopBar badge). */
  number?: number;
  /** Hide the textual label — useful inside dense rows where the icon alone is enough. */
  iconOnly?: boolean;
};

const TONE: Record<AlertSeverity, 'default' | 'caution' | 'fault'> = {
  info: 'default',
  warning: 'caution',
  critical: 'fault',
};

export function AlertChip({ severity, number, iconOnly }: Props) {
  const Icon =
    severity === 'critical' ? AlertOctagon : severity === 'warning' ? AlertTriangle : Info;
  return (
    <Chip
      tone={TONE[severity]}
      icon={<Icon size={12} aria-hidden />}
      number={number}
    >
      {iconOnly ? null : severity}
    </Chip>
  );
}
