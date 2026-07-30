// IReadable over HTTP Range for @mcap/core's indexed reader. Fetches fixed-size
// blocks with a bounded LRU cache; falls back to one full fetch when the server
// ignores Range (stale agent), which the player surfaces as a size warning.
import type { IReadable } from '@mcap/core';

export class EpisodeGoneError extends Error {
  constructor() {
    super('episode no longer available');
  }
}

const DEFAULT_BLOCK = 1 << 20;
const DEFAULT_MAX_BLOCKS = 64;

/** Below this, a full download is not worth warning the operator about. */
export const FULL_FETCH_WARN_BYTES = 8 * 1024 * 1024;

export class RangeReadable implements IReadable {
  private blockSize: number;
  private maxBlocks: number;
  private fileSize: bigint | null = null;
  private blocks = new Map<number, Uint8Array>(); // insertion order = LRU order
  private inflight = new Map<number, Promise<Uint8Array>>();
  private probing: Promise<void> | null = null;
  private full: Uint8Array | null = null;

  constructor(
    private url: string,
    opts?: { blockSize?: number; maxBlocks?: number },
  ) {
    this.blockSize = opts?.blockSize ?? DEFAULT_BLOCK;
    this.maxBlocks = opts?.maxBlocks ?? DEFAULT_MAX_BLOCKS;
  }

  usedFullFetchFallback(): boolean {
    return this.full !== null;
  }

  /** Fallback happened on a file big enough that the operator should know. */
  fullFetchWarning(): boolean {
    return this.full !== null && this.full.length > FULL_FETCH_WARN_BYTES;
  }

  async size(): Promise<bigint> {
    if (this.fileSize === null) await this.probe();
    return this.fileSize!;
  }

  async read(offset: bigint, length: bigint): Promise<Uint8Array> {
    if (this.fileSize === null) await this.probe();
    const start = Number(offset);
    const len = Number(length);
    if (this.full) {
      // Fail the same way the ranged path does; a short buffer misparses later.
      if (start + len > this.full.length) throw new Error('short read past end of episode');
      return this.full.slice(start, start + len);
    }

    const out = new Uint8Array(len);
    let written = 0;
    let block = Math.floor(start / this.blockSize);
    while (written < len) {
      const data = await this.block(block);
      const from = written === 0 ? start - block * this.blockSize : 0;
      const take = Math.min(data.length - from, len - written);
      if (take <= 0) throw new Error('short read past end of episode');
      out.set(data.subarray(from, from + take), written);
      written += take;
      block++;
    }
    return out;
  }

  /** First request doubles as Range support detection. */
  private async probe(): Promise<void> {
    // Concurrent opens must not each fetch the first block.
    this.probing ??= this.probeOnce().finally(() => { this.probing = null; });
    await this.probing;
  }

  private async probeOnce(): Promise<void> {
    const r = await this.fetchRange(0, this.blockSize - 1);
    if (r.status === 206) {
      const m = /\/(\d+)$/.exec(r.headers.get('Content-Range') ?? '');
      if (!m) throw new Error('missing Content-Range on 206');
      this.fileSize = BigInt(m[1]);
      this.remember(0, new Uint8Array(await r.arrayBuffer()));
      return;
    }
    // Server ignored Range: keep the whole body once.
    this.full = new Uint8Array(await r.arrayBuffer());
    this.fileSize = BigInt(this.full.length);
  }

  private async block(idx: number): Promise<Uint8Array> {
    const hit = this.blocks.get(idx);
    if (hit) {
      this.blocks.delete(idx); // refresh LRU position
      this.blocks.set(idx, hit);
      return hit;
    }
    // Overlapping reads of the same block share one fetch.
    const pending = this.inflight.get(idx);
    if (pending) return pending;
    const fetching = this.fetchBlock(idx).finally(() => this.inflight.delete(idx));
    this.inflight.set(idx, fetching);
    return fetching;
  }

  private async fetchBlock(idx: number): Promise<Uint8Array> {
    const start = idx * this.blockSize;
    const end = Math.min(start + this.blockSize, Number(this.fileSize!)) - 1;
    const r = await this.fetchRange(start, end);
    if (r.status !== 206 && r.status !== 200) throw new Error(`range read failed: ${r.status}`);
    const data = new Uint8Array(await r.arrayBuffer());
    this.remember(idx, data);
    return data;
  }

  private remember(idx: number, data: Uint8Array): void {
    this.blocks.set(idx, data);
    while (this.blocks.size > this.maxBlocks) {
      const oldest = this.blocks.keys().next().value as number;
      this.blocks.delete(oldest);
    }
  }

  private async fetchRange(start: number, end: number): Promise<Response> {
    const r = await fetch(this.url, {
      headers: { Range: `bytes=${start}-${end}` },
      credentials: 'include',
    });
    if (r.status === 404 || r.status === 410) throw new EpisodeGoneError();
    if (!r.ok && r.status !== 206) throw new Error(`episode fetch failed: ${r.status}`);
    return r;
  }
}
