import { describe, expect, it, vi, afterEach } from 'vitest';
import { RangeReadable, EpisodeGoneError } from './rangeReader';

const FILE = new Uint8Array(Array.from({ length: 300 }, (_, i) => i % 256));

/** Fake server: honors Range when ranged=true, otherwise always 200 + full body. */
function mockFetch(opts: { ranged: boolean; gone?: boolean; calls?: string[] }) {
  return vi.fn(async (_url: unknown, init?: RequestInit) => {
    const range = (init?.headers as Record<string, string> | undefined)?.Range;
    opts.calls?.push(range ?? 'none');
    if (opts.gone) return new Response('gone', { status: 404 });
    if (opts.ranged && range) {
      const m = /^bytes=(\d+)-(\d+)$/.exec(range)!;
      const [start, end] = [Number(m[1]), Math.min(Number(m[2]), FILE.length - 1)];
      return new Response(FILE.slice(start, end + 1), {
        status: 206,
        headers: { 'Content-Range': `bytes ${start}-${end}/${FILE.length}` },
      });
    }
    return new Response(FILE, { status: 200 });
  });
}

afterEach(() => vi.unstubAllGlobals());

describe('RangeReadable', () => {
  it('reports size from Content-Range and serves exact slices', async () => {
    vi.stubGlobal('fetch', mockFetch({ ranged: true }));
    const r = new RangeReadable('/ep', { blockSize: 64 });
    expect(await r.size()).toBe(300n);
    const got = await r.read(10n, 20n);
    expect(Array.from(got)).toEqual(Array.from(FILE.slice(10, 30)));
    expect(r.usedFullFetchFallback()).toBe(false);
  });

  it('caches blocks so a re-read does not refetch', async () => {
    const calls: string[] = [];
    vi.stubGlobal('fetch', mockFetch({ ranged: true, calls }));
    const r = new RangeReadable('/ep', { blockSize: 64 });
    await r.size();
    await r.read(0n, 10n);
    const before = calls.length;
    await r.read(2n, 5n); // same block
    expect(calls.length).toBe(before);
  });

  it('evicts least-recently-used blocks beyond maxBlocks', async () => {
    const calls: string[] = [];
    vi.stubGlobal('fetch', mockFetch({ ranged: true, calls }));
    const r = new RangeReadable('/ep', { blockSize: 64, maxBlocks: 2 });
    await r.size();
    await r.read(0n, 1n); // block 0
    await r.read(64n, 1n); // block 1
    await r.read(128n, 1n); // block 2, evicts block 0
    const before = calls.length;
    await r.read(0n, 1n); // block 0 must refetch
    expect(calls.length).toBe(before + 1);
  });

  it('falls back to one full fetch when the server ignores Range', async () => {
    vi.stubGlobal('fetch', mockFetch({ ranged: false }));
    const r = new RangeReadable('/ep', { blockSize: 64 });
    expect(await r.size()).toBe(300n);
    expect(r.usedFullFetchFallback()).toBe(true);
    const got = await r.read(100n, 8n);
    expect(Array.from(got)).toEqual(Array.from(FILE.slice(100, 108)));
  });

  it('throws EpisodeGoneError on 404', async () => {
    vi.stubGlobal('fetch', mockFetch({ ranged: true, gone: true }));
    const r = new RangeReadable('/ep', { blockSize: 64 });
    await expect(r.size()).rejects.toBeInstanceOf(EpisodeGoneError);
  });
});
