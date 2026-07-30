import { describe, expect, it } from 'vitest';
import { buildCsv } from './csv';
import type { Sample } from './decoders';

const S = (tNs: bigint, value: number | null): Sample => ({ tNs, value });

describe('buildCsv', () => {
  it('merges series on the timestamp union with blanks for gaps and N/A', () => {
    const series = new Map<string, Sample[]>([
      ['a', [S(1_000_000_000n, 1), S(3_000_000_000n, 3)]],
      ['b', [S(1_000_000_000n, 10), S(2_000_000_000n, null)]],
    ]);
    const csv = buildCsv(series, ['a', 'b'], 0n, 9_000_000_000n);
    const lines = csv.trimEnd().split('\n');
    expect(lines[0]).toBe('time_ns,time_iso,a,b');
    expect(lines[1]).toBe('1000000000,1970-01-01T00:00:01.000Z,1,10');
    expect(lines[2]).toBe('2000000000,1970-01-01T00:00:02.000Z,,');
    expect(lines[3]).toBe('3000000000,1970-01-01T00:00:03.000Z,3,');
    expect(lines).toHaveLength(4);
  });

  it('filters to the requested time range', () => {
    const series = new Map<string, Sample[]>([
      ['a', [S(1_000_000_000n, 1), S(2_000_000_000n, 2), S(3_000_000_000n, 3)]],
    ]);
    const csv = buildCsv(series, ['a'], 1_500_000_000n, 2_500_000_000n);
    const lines = csv.trimEnd().split('\n');
    expect(lines).toHaveLength(2);
    expect(lines[1].startsWith('2000000000,')).toBe(true);
  });

  it('quotes ids containing commas or quotes', () => {
    const series = new Map<string, Sample[]>([['weird,"id', [S(1n, 5)]]]);
    const csv = buildCsv(series, ['weird,"id'], 0n, 10n);
    expect(csv.split('\n')[0]).toBe('time_ns,time_iso,"weird,""id"');
  });
});
