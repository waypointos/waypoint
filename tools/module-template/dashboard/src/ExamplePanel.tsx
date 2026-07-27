// Renders the example module's telemetry. Replace with your module's panel.
// Nullable fields render a muted "N/A" rather than a sentinel, per Waypoint's
// telemetry conventions.
import { useExampleStats } from './useExampleStats';
import styles from './ExamplePanel.module.css';

export function ExamplePanel({ roverId }: { roverId: string }) {
  const { stats } = useExampleStats(roverId);

  const count = stats?.count ?? null;
  const message = stats?.message ?? null;
  const updated = stats?.t ? new Date(Number(stats.t.seconds) * 1000).toLocaleTimeString() : null;

  return (
    <div className={styles.panel} data-testid="panel-m-example">
      <div className={styles.row}>
        <span className={styles.label}>Count</span>
        {count !== null ? (
          <span className={styles.count}>{String(count)}</span>
        ) : (
          <span className={styles.na}>N/A: waiting for first frame</span>
        )}
      </div>
      <div className={styles.row}>
        <span className={styles.label}>Message</span>
        {message ? (
          <span className={styles.value}>{message}</span>
        ) : (
          <span className={styles.na}>N/A: not set in config</span>
        )}
      </div>
      <div className={styles.row}>
        <span className={styles.label}>Updated</span>
        {updated ? (
          <span className={styles.value}>{updated}</span>
        ) : (
          <span className={styles.na}>N/A</span>
        )}
      </div>
    </div>
  );
}
