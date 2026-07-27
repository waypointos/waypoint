// dashboard/src/views/AuditLogView.tsx
import { useMemo, useState } from 'react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Panel } from '@/ui/primitives/Panel';
import { Chip } from '@/ui/primitives/Chip';
import { useFleet } from '@/state/useFleet';
import { useMe } from '@/state/mode';
import { useAudit, type AuditEvent, type AuditFilters } from '@/state/useAudit';
import styles from './AuditLogView.module.css';

type RangeKey = '1h' | '24h' | '7d' | 'all';

const RANGES: { key: RangeKey; label: string; ms?: number }[] = [
  { key: '1h', label: 'Last hour', ms: 60 * 60 * 1000 },
  { key: '24h', label: 'Last 24h', ms: 24 * 60 * 60 * 1000 },
  { key: '7d', label: 'Last 7d', ms: 7 * 24 * 60 * 60 * 1000 },
  { key: 'all', label: 'All' },
];

function sinceFor(range: RangeKey): string | undefined {
  const r = RANGES.find((x) => x.key === range);
  if (!r?.ms) return undefined;
  return new Date(Date.now() - r.ms).toISOString();
}

function relativeFromNow(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return 'N/A';
  const diff = Date.now() - t;
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

// Map kind -> chip tone. Conservative: only flag image/session-open events
// for visual emphasis; everything else stays default.
function chipToneFor(kind: string): 'default' | 'caution' | 'fault' {
  switch (kind) {
    case 'image':
      return 'caution';
    case 'session_open':
      return 'caution';
    default:
      return 'default';
  }
}

// One-line JSON summary, truncated. Full payload inspection is deferred to a
// future expander row.
function payloadSummary(p: Record<string, unknown>): string {
  if (!p || typeof p !== 'object') return 'N/A';
  const keys = Object.keys(p);
  if (keys.length === 0) return 'N/A';
  let s: string;
  try {
    s = JSON.stringify(p);
  } catch {
    return 'N/A';
  }
  return s.length > 80 ? s.slice(0, 77) + '...' : s;
}

export function AuditLogView() {
  const me = useMe();
  const { rovers } = useFleet();

  const [range, setRange] = useState<RangeKey>('24h');
  const [rover, setRover] = useState<string>('');
  // NOTE: actor email is filtered client-side because /api/audit does not
  // accept an actor-email query param. Server-side filter is a future task.
  const [actorQuery, setActorQuery] = useState('');

  const filters: AuditFilters = useMemo(
    () => ({
      rover: rover || undefined,
      since: sinceFor(range),
      limit: 500,
    }),
    [rover, range],
  );

  const { events, loading, error } = useAudit(filters);

  const visible: AuditEvent[] = useMemo(() => {
    const q = actorQuery.trim().toLowerCase();
    if (!q) return events;
    return events.filter((e) => (e.actorEmail || '').toLowerCase().includes(q));
  }, [events, actorQuery]);

  const noteText = loading ? 'loading...' : error ? `error: ${error}` : `${visible.length} entries`;

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// AUDIT</h1>
            <div className={styles.sub}>Audit log</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <Panel title="FILTERS">
            <div className={styles.filters}>
              <label className={styles.field}>
                <span className={styles.fieldLabel}>Rover</span>
                <select
                  className={styles.input}
                  value={rover}
                  onChange={(e) => setRover(e.target.value)}
                >
                  <option value="">{me?.isAdmin ? 'All rovers' : 'Pick a rover'}</option>
                  {rovers.map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.id} — {r.name}
                    </option>
                  ))}
                </select>
              </label>

              <div className={styles.field}>
                <span className={styles.fieldLabel}>Range</span>
                <div className={styles.rangeRow}>
                  {RANGES.map((r) => (
                    <button
                      key={r.key}
                      type="button"
                      className={[
                        styles.rangeBtn,
                        r.key === range && styles.rangeBtnActive,
                      ]
                        .filter(Boolean)
                        .join(' ')}
                      onClick={() => setRange(r.key)}
                    >
                      {r.label}
                    </button>
                  ))}
                </div>
              </div>

              <label className={styles.field}>
                <span className={styles.fieldLabel}>Actor</span>
                <input
                  className={styles.input}
                  type="text"
                  placeholder="email contains..."
                  value={actorQuery}
                  onChange={(e) => setActorQuery(e.target.value)}
                />
              </label>
            </div>
          </Panel>

          <Panel title="EVENTS" note={noteText} style={{ marginTop: 12 }}>
            {visible.length === 0 ? (
              <div className={styles.empty}>
                <span className={styles.naLabel}>N/A</span>
                <span className={styles.naHint}>
                  {loading
                    ? 'fetching events...'
                    : error
                      ? 'unable to load audit log'
                      : 'No events in this range.'}
                </span>
              </div>
            ) : (
              <div className={styles.table} role="table">
                <div className={styles.row + ' ' + styles.head} role="row">
                  <span role="columnheader">Time</span>
                  <span role="columnheader">Rover</span>
                  <span role="columnheader">Source</span>
                  <span role="columnheader">Actor</span>
                  <span role="columnheader">Kind</span>
                  <span role="columnheader">Payload</span>
                </div>
                {visible.map((e) => (
                  <div className={styles.row} role="row" key={e.id}>
                    <span
                      className={styles.time}
                      title={e.occurredAt}
                      role="cell"
                    >
                      {relativeFromNow(e.occurredAt)}
                    </span>
                    <span className={styles.rover} role="cell">
                      {e.roverId || <span className={styles.muted}>N/A</span>}
                    </span>
                    <span className={styles.source} role="cell">
                      {e.source}
                    </span>
                    <span className={styles.actor} role="cell">
                      {e.actorEmail || <span className={styles.muted}>N/A</span>}
                    </span>
                    <span role="cell">
                      <Chip tone={chipToneFor(e.kind)}>{e.kind}</Chip>
                    </span>
                    <span className={styles.payload} role="cell">
                      {payloadSummary(e.payload)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </Panel>
        </section>
      </main>
    </div>
  );
}
