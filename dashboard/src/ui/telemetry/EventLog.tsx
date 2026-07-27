// dashboard/src/ui/telemetry/EventLog.tsx
//
// Session event ticker (mode changes, alerts, connection). Newest first;
// presentational only, callers own the rolling buffer.
import styles from './EventLog.module.css';

export type SessionEvent = {
  atMs: number;
  text: string;
  tone?: 'default' | 'warn' | 'fault';
};

function hhmmss(atMs: number): string {
  const d = new Date(atMs);
  return [d.getHours(), d.getMinutes(), d.getSeconds()]
    .map((n) => String(n).padStart(2, '0'))
    .join(':');
}

export function EventLog({ events, max = 5 }: { events: SessionEvent[]; max?: number }) {
  if (events.length === 0) return null;
  return (
    <div className={styles.log} data-event-log>
      {events.slice(0, max).map((e, i) => (
        <div key={`${e.atMs}-${i}`} className={styles[e.tone ?? 'default']}>
          <span className={styles.t}>{hhmmss(e.atMs)}</span> {e.text}
        </div>
      ))}
    </div>
  );
}
