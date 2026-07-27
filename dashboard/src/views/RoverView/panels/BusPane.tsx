// Bottom pane of the Logs tab: NATS bus activity, kept separate from logs.
// [Monitor | Raw]: Monitor aggregates rate/sparkline per concrete subject;
// Raw shows decoded messages. Clicking a Monitor row drills into Raw.
import { useEffect, useMemo, useRef, useState } from 'react';
import { getBus } from '@/state/nats';
import { decodeBySubject, formatFields, hexPreview, subjectLeaf } from '@/state/subjectTypes';
import { useBusAggregator, sparkChars, type BusEntry } from './useBusAggregator';
import { useRoverContext } from '../RoverContext';
import { Button } from '@/ui/primitives/Button';
import styles from './LogsPanel.module.css';

const MAX_ENTRIES = 1000;
type Mode = 'monitor' | 'raw';

export function BusPane() {
  const { id } = useRoverContext();
  const [entries, setEntries] = useState<BusEntry[]>([]);
  const [mode, setMode] = useState<Mode>('monitor');
  const [paused, setPaused] = useState(false);
  const [subjectFilter, setSubjectFilter] = useState('');
  const [autoscroll, setAutoscroll] = useState(true);
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  useEffect(() => {
    // Reset the subject filter too: a drill-down stores the full concrete
    // subject, which matches nothing (silently emptying the pane) after a
    // rover switch.
    setEntries([]);
    setSubjectFilter('');
    return getBus().subscribe(`waypoint.${id}.>`, (data, subject) => {
      if (pausedRef.current) return;
      setEntries((prev) => {
        const next = [...prev, { tsMs: Date.now(), subject, bytes: data.byteLength, data }];
        if (next.length > MAX_ENTRIES) next.splice(0, next.length - MAX_ENTRIES);
        return next;
      });
    });
  }, [id]);

  const stats = useBusAggregator(entries);
  const filteredStats = useMemo(
    () => stats.filter((s) => !subjectFilter || s.subject.includes(subjectFilter)),
    [stats, subjectFilter],
  );
  const rawRows = useMemo(
    () => entries.filter((e) => !subjectFilter || e.subject.includes(subjectFilter)),
    [entries, subjectFilter],
  );

  const scroller = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (mode === 'raw' && autoscroll && !paused && scroller.current) {
      scroller.current.scrollTop = scroller.current.scrollHeight;
    }
  }, [rawRows, mode, autoscroll, paused]);

  function drillInto(subject: string) {
    setSubjectFilter(subject);
    setMode('raw');
  }

  return (
    <section className={styles.pane} data-testid="bus-pane">
      <header className={styles.paneHeader}>
        <span className={styles.title}>Bus</span>
        <div className={styles.seg}>
          <button className={mode === 'monitor' ? styles.segOn : styles.segOff} onClick={() => setMode('monitor')}>Monitor</button>
          <button className={mode === 'raw' ? styles.segOn : styles.segOff} onClick={() => setMode('raw')}>Raw</button>
        </div>
        {/* Shown only in Raw mode: it represents the drill-down context, not a typed Monitor filter. */}
        {mode === 'raw' && subjectFilter && (
          <span className={styles.crumb} data-testid="raw-breadcrumb">
            ▸ {subjectLeaf(subjectFilter) || subjectFilter}
            <button className={styles.crumbX} onClick={() => setSubjectFilter('')}>✕</button>
          </span>
        )}
        <input
          className={styles.input}
          placeholder="subject…"
          value={subjectFilter}
          onChange={(e) => setSubjectFilter(e.target.value)}
        />
        {mode === 'raw' && (
          <Button size="sm" onClick={() => setAutoscroll((a) => !a)}>
            {autoscroll ? '⤓ follow' : '↧ paused'}
          </Button>
        )}
        <Button size="sm" onClick={() => { setEntries([]); setSubjectFilter(''); }}>clear</Button>
        <Button size="sm" onClick={() => setPaused((p) => !p)}>
          {paused ? 'RESUME' : 'PAUSE'}
        </Button>
      </header>

      {mode === 'monitor' ? (
        <div className={styles.scroller}>
          {filteredStats.length === 0 && <div className={styles.empty}>waiting for bus traffic…</div>}
          {filteredStats.map((s) => (
            <div key={s.subject} className={styles.busRow} onClick={() => drillInto(s.subject)}>
              <span className={styles.subj}>{subjectLeaf(s.subject)}</span>
              <span className={styles.rate}>{s.ratePerSec === null ? '—' : `${round(s.ratePerSec)}/s`}</span>
              <span className={styles.spark}>{sparkChars(s.spark)}</span>
              <span className={styles.sz}>{s.lastBytes}B</span>
            </div>
          ))}
        </div>
      ) : (
        <div className={styles.scroller} ref={scroller}>
          {rawRows.length === 0 && <div className={styles.empty}>waiting for bus traffic…</div>}
          {rawRows.map((e, i) => (
            <div key={i} className={styles.rawRow}>
              <time className={styles.ts}>{formatTs(e.tsMs)}</time>
              <span className={styles.rsubj}>{subjectLeaf(e.subject)}</span>
              <span className={styles.pay}>{previewOf(e)}</span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function previewOf(e: BusEntry): string {
  const decoded = decodeBySubject(e.subject, e.data);
  return decoded ? formatFields(decoded) : hexPreview(e.data);
}

function round(n: number): number {
  return Math.round(n * 10) / 10;
}

function formatTs(ms: number): string {
  return new Date(ms).toISOString().split('T')[1]?.replace('Z', '') ?? '';
}
