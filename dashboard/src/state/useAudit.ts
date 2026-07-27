// dashboard/src/state/useAudit.ts
//
// Polls GET /api/audit on a fixed interval and returns the current event
// window. Filters are passed through to the query string; the backend
// enforces authorization (admin sees everything; non-admin must specify
// a rover they have access to).
import { useEffect, useState } from 'react';

export type AuditEvent = {
  id: string;
  occurredAt: string;
  source: string;
  actorUserId: string;
  actorEmail: string;
  roverId: string;
  kind:
    | 'command'
    | 'access'
    | 'session_open'
    | 'session_close'
    | 'image'
    | string;
  payload: Record<string, unknown>;
};

export type AuditFilters = {
  rover?: string;
  since?: string;
  limit?: number;
};

const POLL_MS = 10_000;

export function useAudit(filters: AuditFilters) {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchOnce() {
      try {
        const q = new URLSearchParams();
        if (filters.rover) q.set('rover', filters.rover);
        if (filters.since) q.set('since', filters.since);
        if (filters.limit) q.set('limit', String(filters.limit));
        const url = q.toString() ? `/api/audit?${q}` : '/api/audit';
        const r = await fetch(url, { credentials: 'include' });
        if (!r.ok) throw new Error(`/api/audit -> ${r.status}`);
        const data = (await r.json()) as AuditEvent[];
        if (!cancelled) {
          setEvents(data ?? []);
          setError(null);
        }
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    fetchOnce();
    const t = window.setInterval(fetchOnce, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(t);
    };
  }, [filters.rover, filters.since, filters.limit]);

  return { events, loading, error };
}
