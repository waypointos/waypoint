// dashboard/src/ui/telemetry/SpeedPresets.tsx
//
// Slow/Cruise/Turbo speed scaling plus a max-speed cap. Purely dashboard-side —
// see state/speed.ts. The cap is a 0..1 fraction; the slider shows percent.
import type { SpeedPreset } from '@/state/speed';
import styles from './SpeedPresets.module.css';

type Props = {
  preset: SpeedPreset;
  cap: number;
  onPreset: (p: SpeedPreset) => void;
  onCap: (c: number) => void;
  disabled?: boolean;
};

const ITEMS: { id: SpeedPreset; label: string }[] = [
  { id: 'slow', label: 'Slow' },
  { id: 'cruise', label: 'Cruise' },
  { id: 'turbo', label: 'Turbo' },
];

export function SpeedPresets({ preset, cap, onPreset, onCap, disabled = false }: Props) {
  return (
    <div className={styles.wrap} data-speed-presets>
      <div className={styles.seg}>
        {ITEMS.map((it) => (
          <button
            key={it.id}
            type="button"
            className={[styles.item, preset === it.id && styles.on].filter(Boolean).join(' ')}
            aria-pressed={preset === it.id}
            disabled={disabled}
            onClick={() => onPreset(it.id)}
          >
            {it.label}
          </button>
        ))}
      </div>
      <label className={styles.cap}>
        <span className={styles.capLbl}>Cap</span>
        <input
          type="range"
          min={0}
          max={100}
          value={Math.round(cap * 100)}
          disabled={disabled}
          onChange={(e) => onCap(Number(e.target.value) / 100)}
        />
        <span className={styles.capVal}>{Math.round(cap * 100)}%</span>
      </label>
    </div>
  );
}
