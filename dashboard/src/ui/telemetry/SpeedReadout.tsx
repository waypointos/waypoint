// dashboard/src/ui/telemetry/SpeedReadout.tsx
//
// Boxed speed cluster for the Teleop HUD: measured body speed large, commanded
// vx and measured yaw beneath. Fixed-width mono values so live updates never
// shift the panel (see ui/format.ts).
import type { SpeedPreset } from '@/state/speed';
import { signedFixed } from '@/ui/format';
import styles from './SpeedReadout.module.css';

type Props = {
  mps: number | null;
  cmdMps?: number | null;
  yawRadps?: number | null;
  preset: SpeedPreset;
  cap: number;
};

export function SpeedReadout({ mps, cmdMps = null, yawRadps = null, preset, cap }: Props) {
  const yawDps = yawRadps == null ? null : (yawRadps * 180) / Math.PI;
  return (
    <div className={styles.wrap} data-speed-readout>
      <div>
        <span className={styles.v}>{mps == null ? 'N/A' : Math.abs(mps).toFixed(2)}</span>
        <span className={styles.u}>m/s</span>
      </div>
      <div className={styles.sub}>
        cmd {cmdMps == null ? '  N/A' : signedFixed(cmdMps)} · yaw{' '}
        {yawDps == null ? '  N/A' : signedFixed(yawDps, 1)}°/s
      </div>
      <div className={styles.sub}>{preset} · cap {Math.round(cap * 100)}%</div>
    </div>
  );
}
