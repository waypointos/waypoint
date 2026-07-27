// dashboard/src/ui/telemetry/ActiveAlertsPanel.tsx
//
// Renders the active alerts for a rover (or the whole fleet, when no rover
// filter is applied). Each row exposes ack/resolve actions when the caller
// indicates the operator has control+ on the displayed rover(s).
//
// Mutations are not handled here — the caller passes onAcknowledge/onResolve
// which typically call the REST endpoints from useAlerts. Local state is
// driven by the NATS replay that lands back through the parent.

import { useState } from 'react';
import { Check, X } from 'lucide-react';
import { Panel } from '@/ui/primitives/Panel';
import { Button } from '@/ui/primitives/Button';
import { AlertChip } from './AlertChip';
import type { ActiveAlert } from '@/state/useAlerts';
import styles from './ActiveAlertsPanel.module.css';

type Props = {
  alerts: ActiveAlert[];
  /** Whether to show ack/resolve buttons. False = read-only render. */
  canControl?: boolean;
  /** Hide rover IDs (useful when the panel is scoped to a single rover). */
  hideRover?: boolean;
  onAcknowledge?: (id: string) => Promise<void> | void;
  onResolve?: (id: string) => Promise<void> | void;
  /** Override the Panel title. */
  title?: string;
};

function relativeFromMs(ms: number): string {
  const diff = Date.now() - ms;
  if (!Number.isFinite(diff)) return 'N/A';
  if (diff < 0) return 'just now';
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

export function ActiveAlertsPanel({
  alerts,
  canControl = false,
  hideRover = false,
  onAcknowledge,
  onResolve,
  title = 'ALERTS',
}: Props) {
  const [busy, setBusy] = useState<string | null>(null);

  const note = alerts.length === 0 ? 'all clear' : `${alerts.length} active`;

  async function run(id: string, fn?: (id: string) => Promise<void> | void) {
    if (!fn) return;
    setBusy(id);
    try {
      await fn(id);
    } catch {
      // Surfacing the error is the parent's job. We unblock the UI either way.
    } finally {
      setBusy((cur) => (cur === id ? null : cur));
    }
  }

  return (
    <Panel title={title} note={note}>
      {alerts.length === 0 ? (
        <div className={styles.empty}>
          <span className={styles.naLabel}>N/A</span>
          <span className={styles.naHint}>No active alerts.</span>
        </div>
      ) : (
        <ul className={styles.list}>
          {alerts.map((a) => {
            const inFlight = busy === a.id;
            const showResolve = a.state === 'active' || a.state === 'acknowledged';
            const showAck = a.state === 'active';
            return (
              <li key={a.id} className={styles.row}>
                <div className={styles.head}>
                  <AlertChip severity={a.severity} />
                  {!hideRover && a.roverId ? (
                    <span className={styles.rover}>{a.roverId}</span>
                  ) : null}
                  <span className={styles.source}>
                    {a.source}
                    {a.code ? <span className={styles.dot}> · </span> : null}
                    {a.code}
                  </span>
                  <span className={styles.time} title={new Date(a.raisedAt).toISOString()}>
                    {relativeFromMs(a.raisedAt)}
                  </span>
                  {a.state === 'acknowledged' ? (
                    <span className={styles.ackd}>ACK&apos;D</span>
                  ) : null}
                </div>
                <div className={styles.msg}>
                  {a.message ? a.message : <span className={styles.muted}>N/A</span>}
                </div>
                {canControl && (showAck || showResolve) ? (
                  <div className={styles.actions}>
                    {showAck ? (
                      <Button
                        icon={<Check size={12} aria-hidden />}
                        onClick={() => run(a.id, onAcknowledge)}
                        disabled={inFlight}
                      >
                        Acknowledge
                      </Button>
                    ) : null}
                    {showResolve ? (
                      <Button
                        icon={<X size={12} aria-hidden />}
                        onClick={() => run(a.id, onResolve)}
                        disabled={inFlight}
                      >
                        Resolve
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </Panel>
  );
}
