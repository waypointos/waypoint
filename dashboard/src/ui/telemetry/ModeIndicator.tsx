// dashboard/src/ui/telemetry/ModeIndicator.tsx
import { Chip } from '../primitives/Chip';

// 'unknown' is the safe default — shown whenever no event.mode has been
// observed (rover offline, never connected, or freshly mounted before the
// first ModeEvent lands). It is NEVER safe to imply the rover is in Manual.
export type Mode = 'manual' | 'safe' | 'autonomous' | 'estop' | 'unknown';

type Props = { mode: Mode };

const LABEL: Record<Mode, string> = {
  manual:     'Manual',
  safe:       'Safe',
  autonomous: 'Auto',
  estop:      'E-Stop',
  unknown:    'Unknown',
};

const TONE: Record<Mode, 'default' | 'caution' | 'fault'> = {
  manual:     'default',
  safe:       'caution',
  autonomous: 'default',
  estop:      'fault',
  unknown:    'caution',
};

export function ModeIndicator({ mode }: Props) {
  return <Chip dot tone={TONE[mode]}>{LABEL[mode]}</Chip>;
}
