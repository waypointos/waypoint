// dashboard/src/lib/episode/csv.ts
//
// Wide CSV: one row per timestamp in the union of the selected series, blank
// cells where a series has no sample or the sample is N/A.
import type { Sample } from './decoders';

function field(s: string): string {
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function buildCsv(
  series: Map<string, Sample[]>,
  ids: string[],
  startNs: bigint,
  endNs: bigint,
): string {
  const byIdByTime = new Map<string, Map<bigint, number | null>>();
  const times = new Set<bigint>();
  for (const id of ids) {
    const m = new Map<bigint, number | null>();
    for (const s of series.get(id) ?? []) {
      if (s.tNs < startNs || s.tNs > endNs) continue;
      m.set(s.tNs, s.value);
      times.add(s.tNs);
    }
    byIdByTime.set(id, m);
  }
  const ordered = [...times].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  const lines = [['time_ns', 'time_iso', ...ids.map(field)].join(',')];
  for (const t of ordered) {
    const iso = new Date(Number(t / 1_000_000n)).toISOString();
    const cells = ids.map((id) => {
      const v = byIdByTime.get(id)?.get(t);
      return v === undefined || v === null ? '' : String(v);
    });
    lines.push([t.toString(), iso, ...cells].join(','));
  }
  return lines.join('\n') + '\n';
}
