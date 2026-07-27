// dashboard/src/ui/telemetry/MotorCard.tsx
import styles from './MotorCard.module.css';

type Props = {
  /** Servo ID, e.g. 7 */
  id: number;
  /** Short label, e.g. "BL" (back-left). Omit when the surface has no
   *  platform/module-level knowledge of the slot's role. */
  position?: string;
  /** Amps. null → "—". */
  currentA: number | null;
  /** °C. null → "—". */
  temperatureC: number | null;
  /** Threshold above which the card turns amber. */
  hotAtC?: number;
  /**
   * Commanded angular velocity in rad/s for this motor — what the dashboard
   * is asking core to drive the wheel at. Undefined hides the line entirely
   * (use that for non-drive servos). null renders as "—" (drive servo with
   * gating in effect, e.g. monitor / offline / non-manual).
   */
  targetVelRadps?: number | null;
  /**
   * Last reported angular velocity from telemetry.motors, rad/s. Same null
   * semantics as targetVelRadps.
   */
  actualVelRadps?: number | null;
};

export function MotorCard({
  id,
  position,
  currentA,
  temperatureC,
  hotAtC = 55,
  targetVelRadps,
  actualVelRadps,
}: Props) {
  const hot = temperatureC != null && temperatureC >= hotAtC;
  const showVel = targetVelRadps !== undefined || actualVelRadps !== undefined;
  return (
    <div className={`${styles.card} ${hot ? styles.hot : ''}`}>
      <div className={styles.id}>
        {String(id).padStart(2, '0')}
        {position && <><br />{position}</>}
      </div>
      <div className={styles.vals}>
        <div className={styles.a}>
          {currentA == null ? '—' : currentA.toFixed(1)}
          <span className={styles.u}>A</span>
        </div>
        <div className={styles.t}>
          {temperatureC == null ? '—' : `${temperatureC.toFixed(0)} °C`}
        </div>
        {showVel && (
          <div className={styles.vel}>
            <span className={styles.velLbl}>tgt</span>
            <span className={styles.velVal}>
              {targetVelRadps == null ? '—' : targetVelRadps.toFixed(1)}
            </span>
            <span className={styles.velLbl}>act</span>
            <span className={styles.velVal}>
              {actualVelRadps == null ? '—' : actualVelRadps.toFixed(1)}
            </span>
            <span className={styles.velUnit}>rad/s</span>
          </div>
        )}
      </div>
    </div>
  );
}
