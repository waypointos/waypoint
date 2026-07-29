import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import type { IReadable } from '@mcap/core';
import { EpisodeSource } from './episodeSource';

const FIXTURE = new Uint8Array(readFileSync(resolve(__dirname, '__fixtures__/episode.mcap')));

/** In-memory IReadable; index/chunk access goes through the same interface
 * RangeReadable implements, without needing a fetch mock. */
class MemReadable implements IReadable {
  async size() { return BigInt(FIXTURE.length); }
  async read(offset: bigint, length: bigint) {
    return FIXTURE.slice(Number(offset), Number(offset) + Number(length));
  }
}

describe('EpisodeSource', () => {
  it('lists channels with kind, count, and rate', async () => {
    const src = await EpisodeSource.open(new MemReadable());
    const byTopic = Object.fromEntries(src.channels().map((c) => [c.topic, c]));
    expect(byTopic['module.drill.sensor.state'].kind).toBe('plot');
    expect(byTopic['module.drill.sensor.state'].count).toBe(3);
    expect(byTopic['module.drill.drill.state'].kind).toBe('schemaless');
    expect(byTopic['camera.cam0/h264'].kind).toBe('video');
    // 3 messages across t=1s..3s: 1 Hz.
    expect(byTopic['telemetry.motors'].rateHz).toBeCloseTo(1, 1);
  });

  it('reports the episode time range', async () => {
    const src = await EpisodeSource.open(new MemReadable());
    const { startNs, endNs } = src.timeRange();
    expect(startNs).toBe(1_000_000_000n);
    expect(endNs).toBe(3_000_000_000n);
  });

  it('decodes sensor series with N/A gaps', async () => {
    const src = await EpisodeSource.open(new MemReadable());
    const all = await src.series(['module.drill.sensor.state'], 0n, 4_000_000_000n);
    const a = all.get('module.drill.sensor.state.cell_a_g')!;
    expect(a.map((s) => s.value)).toEqual([100, 110, 120]);
    const b = all.get('module.drill.sensor.state.cell_b_g')!;
    expect(b.map((s) => s.value)).toEqual([200, null, 220]);
  });

  it('keys motor series by servo id', async () => {
    const src = await EpisodeSource.open(new MemReadable());
    const all = await src.series(['telemetry.motors'], 0n, 4_000_000_000n);
    expect(all.get('telemetry.motors.1.velocityRadps')!.map((s) => s.value))
      .toEqual([0.5, 0.6, 0.7]);
  });

  it('respects the time window', async () => {
    const src = await EpisodeSource.open(new MemReadable());
    const all = await src.series(['module.drill.sensor.state'], 1_500_000_000n, 4_000_000_000n);
    expect(all.get('module.drill.sensor.state.cell_a_g')!.map((s) => s.value)).toEqual([110, 120]);
  });

  it('returns raw video AUs', async () => {
    const src = await EpisodeSource.open(new MemReadable());
    const aus = await src.videoAUs('camera.cam0/h264', 0n, 4_000_000_000n);
    expect(aus).toHaveLength(2);
    expect(Array.from(aus[0].data)).toEqual([0, 0, 0, 1, 0x67, 0x42, 0, 0x1e]);
  });
});
