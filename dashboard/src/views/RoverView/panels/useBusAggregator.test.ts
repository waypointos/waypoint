import { describe, it, expect } from 'vitest';
import { aggregate, sparkChars, type BusEntry } from './useBusAggregator';

function entry(subject: string, tsMs: number, bytes = 10): BusEntry {
  return { tsMs, subject, bytes, data: new Uint8Array(bytes) };
}

describe('aggregate', () => {
  const now = 10_000;

  it('returns an empty array for no entries', () => {
    expect(aggregate([], now, 1000, 12, 1000)).toEqual([]);
  });

  it('computes per-subject rate over the window and sorts by subject', () => {
    const entries: BusEntry[] = [
      entry('waypoint.r.telemetry.power', now - 100),
      entry('waypoint.r.telemetry.power', now - 200),
      entry('waypoint.r.heartbeat', now - 300),
    ];
    const stats = aggregate(entries, now, 1000, 12, 1000);
    expect(stats.map((s) => s.subject)).toEqual([
      'waypoint.r.heartbeat',
      'waypoint.r.telemetry.power',
    ]);
    const power = stats.find((s) => s.subject === 'waypoint.r.telemetry.power')!;
    expect(power.ratePerSec).toBe(2); // 2 msgs in a 1000ms window = 2/s
    expect(power.lastBytes).toBe(10);
  });

  it('reports null rate (N/A) when the subject is stale beyond the window', () => {
    const stats = aggregate([entry('waypoint.r.event.arm', now - 5000)], now, 1000, 12, 1000);
    expect(stats[0].ratePerSec).toBeNull();
  });

  it('buckets recent activity into the sparkline (newest on the right)', () => {
    const entries: BusEntry[] = [
      entry('waypoint.r.x', now - 500), // bucket 11 (newest)
      entry('waypoint.r.x', now - 1500), // bucket 10
      entry('waypoint.r.x', now - 1600), // bucket 10
    ];
    const stats = aggregate(entries, now, 1000, 12, 1000);
    expect(stats[0].spark[11]).toBe(1);
    expect(stats[0].spark[10]).toBe(2);
    expect(stats[0].spark[0]).toBe(0);
  });
});

describe('sparkChars', () => {
  it('maps counts to block glyphs scaled to the max', () => {
    expect(sparkChars([0, 1, 2, 4])).toBe(' ▂▄█');
  });
  it('handles all-zero input without dividing by zero', () => {
    expect(sparkChars([0, 0, 0])).toBe('   ');
  });
});
