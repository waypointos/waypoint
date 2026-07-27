// dashboard/src/ui/telemetry/Reticle.tsx
// Fixed FPV crosshair (amber, per the HUD-reticle design rule). No attitude
// data yet (no IMU), so the brackets are a static aiming aid, not an horizon.
// The centre dot drifts with commanded motion as a steering-intent cue.
import styles from './Reticle.module.css';

const DRIFT_PX = 30;

export function Reticle({ drift }: { drift?: { vx: number; yaw: number } | null }) {
  const dotStyle = drift
    ? { transform: `translate(calc(-50% + ${drift.yaw * DRIFT_PX}px), calc(-50% + ${-drift.vx * DRIFT_PX}px))` }
    : undefined;
  return (
    <div className={styles.reticle} aria-hidden>
      <span className={`${styles.b} ${styles.tl}`} />
      <span className={`${styles.b} ${styles.tr}`} />
      <span className={`${styles.b} ${styles.bl}`} />
      <span className={`${styles.b} ${styles.br}`} />
      <span className={styles.dot} style={dotStyle} />
    </div>
  );
}
