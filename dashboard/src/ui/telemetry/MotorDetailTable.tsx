// Dense, debug-flavoured table of the live MotorTelemetry stream for the
// drive servos. Pair with the MotorCard grid as the at-a-glance view: this
// table is the raw-data companion.
//
// The `target` column is optional. Include it on surfaces that own the drive
// command (so the operator can compare commanded vs measured); omit it on
// monitor surfaces where target would always be 0 / not-applicable.
import styles from './MotorDetailTable.module.css';

type MotorSample = {
  positionRad?: number | null;
  velocityRadps?: number | null;
  currentA?: number | null;
  temperatureC?: number | null;
  busVoltageV?: number | null;
};

type Props = {
  motors: Record<number, MotorSample>;
  ids: ReadonlyArray<number>;
  /**
   * Optional friendly slot labels per motor id (e.g. {7:'BL'}). Omit on
   * surfaces that have no platform-level knowledge of slot roles — the
   * column will show the bare id only.
   */
  positions?: Record<number, string>;
  /** Map from servo id to commanded rad/s. Omit to hide the column. */
  targets?: Record<number, number>;
};

export function MotorDetailTable({ motors, ids, positions, targets }: Props) {
  const showTarget = targets !== undefined;
  return (
    <table className={styles.detail}>
      <thead>
        <tr>
          <th>ID</th>
          <th>Pos</th>
          {showTarget && <th>Tgt</th>}
          <th>Act</th>
          <th>I</th>
          <th>°C</th>
        </tr>
      </thead>
      <tbody>
        {ids.map((id) => {
          const m = motors[id];
          const tgt = targets?.[id];
          return (
            <tr key={id}>
              <td className={styles.detailId}>
                {String(id).padStart(2, '0')}{positions?.[id] ? ` · ${positions[id]}` : ''}
              </td>
              <td>{m?.positionRad   == null ? '—' : m.positionRad.toFixed(3)}</td>
              {showTarget && (
                <td>{tgt == null ? '—' : tgt.toFixed(2)}</td>
              )}
              <td>{m?.velocityRadps == null ? '—' : m.velocityRadps.toFixed(2)}</td>
              <td>{m?.currentA      == null ? '—' : m.currentA.toFixed(2)}</td>
              <td>{m?.temperatureC  == null ? '—' : m.temperatureC.toFixed(0)}</td>
            </tr>
          );
        })}
      </tbody>
      <tfoot>
        <tr>
          <td colSpan={showTarget ? 6 : 5} className={styles.detailFoot}>
            Bus: {firstBusVoltage(motors, ids)} · units: pos rad · vel rad/s · I A
          </td>
        </tr>
      </tfoot>
    </table>
  );
}

function firstBusVoltage(motors: Record<number, MotorSample>, ids: ReadonlyArray<number>): string {
  for (const id of ids) {
    const v = motors[id]?.busVoltageV;
    if (v != null) return `${v.toFixed(2)} V`;
  }
  return '— V';
}
