// dashboard/src/ui/telemetry/ModeControl.tsx
//
// Clickable Manual/Safe segmented control. Mode is authoritative from core;
// this only requests transitions. While estopped, selection is disabled — the
// operator must Clear the E-stop first.
import type { Mode as DisplayMode } from './ModeIndicator';
import type { PendingIntent } from '@/state/useModeCommand';
import styles from './ModeControl.module.css';

type Target = 'manual' | 'safe';
type Props = {
  mode: DisplayMode;
  pending: PendingIntent;
  disabled?: boolean;
  /** 'sm' is the compact inline variant for the teleop strip; 'md' fills its container (Control tab). */
  size?: 'md' | 'sm';
  onSelect: (target: Target) => void;
};

const ITEMS: { id: Target; label: string }[] = [
  { id: 'manual', label: 'Manual' },
  { id: 'safe', label: 'Safe' },
];

export function ModeControl({ mode, pending, disabled = false, size = 'md', onSelect }: Props) {
  const estopped = mode === 'estop';
  return (
    <div
      className={`${styles.seg} ${size === 'sm' ? styles.sm : ''}`}
      role="group"
      aria-label="Drive mode"
      data-mode-control
    >
      {ITEMS.map((it) => {
        const active = mode === it.id;
        const isPending = pending === it.id;
        return (
          <button
            key={it.id}
            type="button"
            className={[styles.item, active && styles.on, isPending && styles.pending].filter(Boolean).join(' ')}
            aria-pressed={active}
            data-pending={isPending || undefined}
            disabled={disabled || estopped}
            onClick={() => onSelect(it.id)}
          >
            {it.label}
          </button>
        );
      })}
    </div>
  );
}
