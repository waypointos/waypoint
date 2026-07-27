import { useEffect, useMemo, useState } from 'react';

export type BusEntry = {
  tsMs: number;
  subject: string; // concrete subject
  bytes: number;
  data: Uint8Array;
};

export type SubjectStat = {
  subject: string;
  ratePerSec: number | null; // null = stale (first-class N/A)
  lastBytes: number;
  spark: number[]; // per-bucket counts, newest on the right
};

const BLOCKS = ' ▁▂▃▄▅▆▇█';

/** Pure aggregation: group entries by subject, compute rate over `windowMs`
 * and a `buckets`-wide activity sparkline. Stale subjects get null rate. */
export function aggregate(
  entries: BusEntry[],
  now: number,
  windowMs: number,
  buckets: number,
  bucketMs: number,
): SubjectStat[] {
  const bySubject = new Map<string, BusEntry[]>();
  for (const e of entries) {
    let list = bySubject.get(e.subject);
    if (!list) bySubject.set(e.subject, (list = []));
    list.push(e);
  }

  const out: SubjectStat[] = [];
  for (const [subject, list] of bySubject) {
    // Most recent by timestamp, not arrival order, so unsorted input is safe.
    const last = list.reduce((a, b) => (a.tsMs > b.tsMs ? a : b));
    const stale = now - last.tsMs > windowMs;
    const recent = list.filter((e) => now - e.tsMs <= windowMs).length;
    const spark = new Array<number>(buckets).fill(0);
    for (const e of list) {
      const idx = buckets - 1 - Math.floor((now - e.tsMs) / bucketMs);
      if (idx >= 0 && idx < buckets) spark[idx]++;
    }
    out.push({
      subject,
      ratePerSec: stale ? null : recent / (windowMs / 1000),
      lastBytes: last.bytes,
      spark,
    });
  }
  out.sort((a, b) => a.subject.localeCompare(b.subject));
  return out;
}

/** Render bucket counts as a unicode block sparkline scaled to the max. */
export function sparkChars(spark: number[]): string {
  const max = Math.max(0, ...spark);
  if (max === 0) return ' '.repeat(spark.length);
  return spark.map((v) => BLOCKS[Math.round((v / max) * (BLOCKS.length - 1))]).join('');
}

/** React hook: re-aggregates `entries` once per second so rates decay to N/A
 * even when no new messages arrive. */
export function useBusAggregator(
  entries: BusEntry[],
  windowMs = 1000,
  buckets = 12,
  bucketMs = 1000,
): SubjectStat[] {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setTick((x) => x + 1), 1000);
    return () => clearInterval(t);
  }, []);
  // tick is intentionally a dependency: it forces recompute against a fresh
  // Date.now() so stale subjects transition to null rate on a timer.
  return useMemo(
    () => aggregate(entries, Date.now(), windowMs, buckets, bucketMs),
    [entries, tick, windowMs, buckets, bucketMs],
  );
}
