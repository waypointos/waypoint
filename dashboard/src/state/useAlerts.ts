// dashboard/src/state/useAlerts.ts
//
// Live view of the active-alert set across every rover the operator can see.
//
// Two sources keep the in-memory map fresh:
//   1) GET /api/alerts on mount — seeds rows that were raised before the
//      dashboard mounted (NATS subscriptions don't replay history).
//   2) waypoint.<rover>.alert.{raised,acknowledged,resolved} subscriptions —
//      converge subsequent state.
//
// The hook is read-only; mutation goes through the action functions
// acknowledgeAlert/resolveAlert, which hit the REST API and let the resulting
// NATS event update local state. We do not optimistically mutate, which keeps
// a single source of truth.

import { useEffect, useMemo, useState } from 'react';
import { getBus } from './nats';
import { useFleet } from './useFleet';
import { useMe } from './mode';
import { pbFromBinary } from './protobuf';
import {
  AlertAcknowledged,
  AlertRaised,
  AlertResolved,
} from '../../../protocol/gen/ts/messages/alerts_pb';
import { Severity } from '../../../protocol/gen/ts/messages/common_pb';

// Severity domain: matches the Postgres alert_severity enum + the dashboard's
// UI tone mapping. The proto enum (SEVERITY_INFO=1, SEVERITY_WARN=2,
// SEVERITY_FAULT=3) is normalised to these strings on entry.
export type AlertSeverity = 'info' | 'warning' | 'critical';

export type AlertState = 'active' | 'acknowledged';

export type ActiveAlert = {
  id: string;
  roverId: string;
  severity: AlertSeverity;
  source: string;
  code: string;
  message: string;
  metadata?: Record<string, string>;
  raisedAt: number; // unix ms
  state: AlertState;
};

export type AlertCounts = {
  info: number;
  warning: number;
  critical: number;
};

// JSON shape returned by GET /api/alerts (mirrors proxy/internal/api/alerts.go
// `alertWireRow`). RaisedAt is RFC3339; metadata is the raw JSONB or omitted.
type AlertWireRow = {
  id: string;
  roverId: string;
  severity: string;
  source: string;
  code: string;
  message: string;
  metadata?: Record<string, string> | null;
  raisedAt: string;
  state: string;
  acknowledgedAt?: string;
  acknowledgedBy?: string;
  resolvedAt?: string;
  resolvedBy?: string;
  autoResolved?: boolean;
};

function sevFromProto(s: number): AlertSeverity {
  if (s === Severity.WARN) return 'warning';
  if (s === Severity.FAULT) return 'critical';
  return 'info';
}

function sevFromWire(s: string): AlertSeverity {
  if (s === 'warning' || s === 'warn') return 'warning';
  if (s === 'critical' || s === 'fault') return 'critical';
  return 'info';
}

function rowToAlert(row: AlertWireRow): ActiveAlert {
  const t = Date.parse(row.raisedAt);
  return {
    id: row.id,
    roverId: row.roverId,
    severity: sevFromWire(row.severity),
    source: row.source,
    code: row.code,
    message: row.message,
    metadata: row.metadata ?? undefined,
    raisedAt: Number.isFinite(t) ? t : Date.now(),
    state: row.state === 'acknowledged' ? 'acknowledged' : 'active',
  };
}

export function useAlerts(): {
  alerts: ActiveAlert[];
  counts: AlertCounts;
  loading: boolean;
  error: string | null;
} {
  const me = useMe();
  const { rovers } = useFleet();
  const [byID, setByID] = useState<Map<string, ActiveAlert>>(new Map());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // One-shot REST seed on mount. Keeps the dashboard accurate if the user
  // loads the page after `raised` events were missed by the WebSocket.
  // /api/alerts is proxy-only; off-proxy we rely on the NATS alert.* events
  // alone, so skip the seed rather than hammering a 404 on the local agent.
  useEffect(() => {
    if (me?.mode !== 'proxy') {
      setLoading(false);
      return;
    }
    let cancelled = false;
    async function seed() {
      try {
        const r = await fetch('/api/alerts', { credentials: 'include' });
        if (!r.ok) {
          // 403 with no rover scope is normal for non-admins; treat as empty.
          if (!cancelled) setError(`/api/alerts -> ${r.status}`);
          return;
        }
        const rows = (await r.json()) as AlertWireRow[] | null;
        if (cancelled || !rows) return;
        setByID((prev) => {
          const next = new Map(prev);
          for (const row of rows) {
            if (row.state === 'resolved') continue;
            next.set(row.id, rowToAlert(row));
          }
          return next;
        });
        if (!cancelled) setError(null);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    seed();
    return () => {
      cancelled = true;
    };
  }, [me?.mode]);

  // NATS subscriptions per rover. Re-bind when the rover list changes.
  // The bus de-duplicates subscriptions, so re-binding the same subject is
  // a cheap no-op on identical fleets.
  const roverKey = rovers.map((r) => r.id).join(',');
  useEffect(() => {
    if (rovers.length === 0) return;
    const bus = getBus();
    const unsubs: Array<() => void> = [];

    for (const r of rovers) {
      unsubs.push(
        bus.subscribe(`waypoint.${r.id}.alert.raised`, (bytes) => {
          try {
            const ev = pbFromBinary<AlertRaised>(AlertRaised, bytes);
            setByID((prev) => {
              const next = new Map(prev);
              const tSec = ev.t?.seconds ? Number(ev.t.seconds) : 0;
              next.set(ev.id, {
                id: ev.id,
                roverId: r.id,
                severity: sevFromProto(ev.severity),
                source: ev.source,
                code: ev.code,
                message: ev.message,
                metadata:
                  ev.metadata && Object.keys(ev.metadata).length > 0
                    ? { ...ev.metadata }
                    : undefined,
                raisedAt: tSec > 0 ? tSec * 1000 : Date.now(),
                state: 'active',
              });
              return next;
            });
          } catch {
            // ignore decode errors
          }
        }),
      );
      unsubs.push(
        bus.subscribe(`waypoint.${r.id}.alert.acknowledged`, (bytes) => {
          try {
            const ev = pbFromBinary<AlertAcknowledged>(AlertAcknowledged, bytes);
            setByID((prev) => {
              const cur = prev.get(ev.id);
              if (!cur || cur.state === 'acknowledged') return prev;
              const next = new Map(prev);
              next.set(ev.id, { ...cur, state: 'acknowledged' });
              return next;
            });
          } catch {
            // ignore decode errors
          }
        }),
      );
      unsubs.push(
        bus.subscribe(`waypoint.${r.id}.alert.resolved`, (bytes) => {
          try {
            const ev = pbFromBinary<AlertResolved>(AlertResolved, bytes);
            setByID((prev) => {
              if (!prev.has(ev.id)) return prev;
              const next = new Map(prev);
              next.delete(ev.id);
              return next;
            });
          } catch {
            // ignore decode errors
          }
        }),
      );
    }

    return () => {
      for (const off of unsubs) off();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roverKey]);

  const alerts = useMemo(() => {
    return Array.from(byID.values()).sort((a, b) => b.raisedAt - a.raisedAt);
  }, [byID]);

  const counts = useMemo<AlertCounts>(() => {
    const c: AlertCounts = { info: 0, warning: 0, critical: 0 };
    for (const a of alerts) c[a.severity] += 1;
    return c;
  }, [alerts]);

  return { alerts, counts, loading, error };
}

/** POST /api/alerts/{id}/acknowledge. Throws on non-2xx. */
export async function acknowledgeAlert(id: string): Promise<void> {
  const r = await fetch(`/api/alerts/${encodeURIComponent(id)}/acknowledge`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!r.ok) throw new Error(`acknowledge -> ${r.status}`);
}

/** POST /api/alerts/{id}/resolve. Throws on non-2xx. */
export async function resolveAlert(id: string): Promise<void> {
  const r = await fetch(`/api/alerts/${encodeURIComponent(id)}/resolve`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!r.ok) throw new Error(`resolve -> ${r.status}`);
}
