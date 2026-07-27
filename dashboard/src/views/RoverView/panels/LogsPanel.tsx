// Logs tab: a readable log stream (top) and a separate NATS bus pane (bottom).
import { LogsStream } from './LogsStream';
import { BusPane } from './BusPane';
import styles from './LogsPanel.module.css';

export function LogsPanel() {
  return (
    <div className={styles.layout} data-testid="panel-logs">
      <LogsStream />
      <BusPane />
    </div>
  );
}
