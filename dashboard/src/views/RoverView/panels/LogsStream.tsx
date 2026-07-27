// Top pane of the Logs tab: the agent/module log stream (waypoint.<id>.diag.log).
import { useEffect, useMemo, useRef, useState } from 'react';
import { getBus } from '@/state/nats';
import { pbFromBinary } from '@/state/protobuf';
import { LogRecord } from '../../../../../protocol/gen/ts/messages/diag_pb';
import { useRoverContext } from '../RoverContext';
import { Button } from '@/ui/primitives/Button';
import styles from './LogsPanel.module.css';

const MAX_ENTRIES = 1000;
const LEVELS = ['debug', 'info', 'warn', 'error'] as const;
type Level = (typeof LEVELS)[number];
const RANK: Record<Level, number> = { debug: 0, info: 1, warn: 2, error: 3 };

type LogEntry = { tsNs: number; level: string; msg: string; source: string };

export function LogsStream() {
  const { id } = useRoverContext();
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [paused, setPaused] = useState(false);
  const [level, setLevel] = useState<Level>('info');
  const [source, setSource] = useState('');
  const [grep, setGrep] = useState('');
  const [autoscroll, setAutoscroll] = useState(true);
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  useEffect(() => {
    // Reset the source filter too: its options derive from entries, so a stale
    // value would silently hide every new log after a rover switch.
    setEntries([]);
    setSource('');
    return getBus().subscribe(`waypoint.${id}.diag.log`, (bytes) => {
      if (pausedRef.current) return;
      try {
        const rec = pbFromBinary<LogRecord>(LogRecord, bytes);
        const entry: LogEntry = {
          tsNs: Number(rec.tsNs),
          level: rec.level,
          msg: rec.msg.replace(/\s+$/, ''), // drop trailing whitespace/newlines
          source: rec.source ?? '',
        };
        setEntries((prev) => {
          const next = [...prev, entry];
          if (next.length > MAX_ENTRIES) next.splice(0, next.length - MAX_ENTRIES);
          return next;
        });
      } catch { /* ignore decode errors */ }
    });
  }, [id]);

  const sources = useMemo(
    () => [...new Set(entries.map((e) => e.source).filter(Boolean))].sort(),
    [entries],
  );

  const filtered = useMemo(() => {
    const min = RANK[level];
    const needle = grep.toLowerCase();
    return entries.filter((e) => {
      if ((RANK[e.level as Level] ?? 0) < min) return false;
      if (source && e.source !== source) return false;
      if (needle && !e.msg.toLowerCase().includes(needle)) return false;
      return true;
    });
  }, [entries, level, source, grep]);

  const scroller = useRef<HTMLOListElement>(null);
  useEffect(() => {
    if (autoscroll && !paused && scroller.current) {
      scroller.current.scrollTop = scroller.current.scrollHeight;
    }
  }, [filtered, autoscroll, paused]);

  return (
    <section className={styles.pane} data-testid="logs-stream">
      <header className={styles.paneHeader}>
        <span className={styles.title}>Logs</span>
        <div className={styles.pills}>
          {LEVELS.map((lv) => (
            <button
              key={lv}
              className={`${styles.pill} ${level === lv ? styles.pillOn : ''} ${styles[`pill-${lv}`] ?? ''}`}
              onClick={() => setLevel(lv)}
            >
              {lv.slice(0, 3).toUpperCase()}
            </button>
          ))}
        </div>
        <select className={styles.select} value={source} onChange={(e) => setSource(e.target.value)}>
          <option value="">all sources</option>
          {sources.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
        <input className={styles.input} placeholder="grep…" value={grep} onChange={(e) => setGrep(e.target.value)} />
        <Button size="sm" onClick={() => setAutoscroll((a) => !a)}>
          {autoscroll ? '⤓ follow' : '↧ paused'}
        </Button>
        <Button size="sm" onClick={() => { setEntries([]); setSource(''); }}>clear</Button>
        <Button size="sm" onClick={() => setPaused((p) => !p)}>
          {paused ? 'RESUME' : 'PAUSE'}
        </Button>
      </header>
      <ol className={styles.scroller} ref={scroller}>
        {filtered.length === 0 && <li className={styles.empty}>waiting for logs…</li>}
        {filtered.map((e, i) => (
          <li key={i} className={`${styles.row} ${styles[`lvl-${e.level}`] ?? ''}`}>
            <time className={styles.ts}>{formatTs(e.tsNs / 1e6)}</time>
            <span className={styles.lvl}>{e.level}</span>
            <span className={styles.src}>{e.source}</span>
            <span className={styles.msg}>{e.msg}</span>
          </li>
        ))}
      </ol>
    </section>
  );
}

function formatTs(ms: number): string {
  return new Date(ms).toISOString().split('T')[1]?.replace('Z', '') ?? '';
}
