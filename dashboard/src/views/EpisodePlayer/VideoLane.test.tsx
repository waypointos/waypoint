import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import type { EpisodeSource } from '../../lib/episode/episodeSource';
import { VideoLane } from './VideoLane';

// Annex-B AUs: the keyframe carries SPS (type 7) plus an IDR slice (type 5).
const KEY = new Uint8Array([0, 0, 1, 0x67, 0x42, 0x00, 0x1e, 0, 0, 1, 0x65]);
const DELTA = new Uint8Array([0, 0, 1, 0x41]);
const S = 1_000_000_000n;

type Chunk = { type: string; timestamp: number };
type Rec = { chunks: Chunk[]; closed: boolean };
const decoders: Rec[] = [];

class FakeDecoder {
  private rec: Rec = { chunks: [], closed: false };
  constructor() { decoders.push(this.rec); }
  configure() {}
  decode(chunk: Chunk) { this.rec.chunks.push(chunk); }
  close() { this.rec.closed = true; }
}

class FakeChunk {
  type: string;
  timestamp: number;
  constructor(init: Chunk) {
    this.type = init.type;
    this.timestamp = init.timestamp;
  }
}

const source = {
  videoAUs: async () => [
    { tNs: 1n * S, data: KEY },
    { tNs: 2n * S, data: DELTA },
    { tNs: 3n * S, data: DELTA },
  ],
} as unknown as EpisodeSource;

function renderAt(cursorNs: bigint) {
  return render(
    <VideoLane source={source} topic="camera.cam0/h264" cursorNs={cursorNs} startNs={S} endNs={3n * S} />,
  );
}

describe('VideoLane', () => {
  beforeEach(() => {
    decoders.length = 0;
    vi.stubGlobal('VideoDecoder', FakeDecoder);
    vi.stubGlobal('EncodedVideoChunk', FakeChunk);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('feeds forward AUs once and re-decodes from the keyframe on a backward seek', async () => {
    const { rerender } = renderAt(1n * S);
    await waitFor(() => expect(decoders).toHaveLength(1));
    expect(decoders[0].chunks.map((c) => c.timestamp)).toEqual([1_000_000]);

    rerender(
      <VideoLane source={source} topic="camera.cam0/h264" cursorNs={3n * S} startNs={S} endNs={3n * S} />,
    );
    // Forward inside the run feeds each AU exactly once.
    expect(decoders).toHaveLength(1);
    expect(decoders[0].chunks.map((c) => c.timestamp)).toEqual([1_000_000, 2_000_000, 3_000_000]);

    rerender(
      <VideoLane source={source} topic="camera.cam0/h264" cursorNs={2n * S} startNs={S} endNs={3n * S} />,
    );
    // Backward inside the same keyframe run: fresh decoder, keyframe first.
    expect(decoders).toHaveLength(2);
    expect(decoders[0].closed).toBe(true);
    expect(decoders[1].chunks.map((c) => c.type)).toEqual(['key', 'delta']);
    expect(decoders[1].chunks.map((c) => c.timestamp)).toEqual([1_000_000, 2_000_000]);

    rerender(
      <VideoLane source={source} topic="camera.cam0/h264" cursorNs={3n * S} startNs={S} endNs={3n * S} />,
    );
    // Moving on after the reset must not replay what the new decoder already has.
    expect(decoders).toHaveLength(2);
    expect(decoders[1].chunks.map((c) => c.timestamp)).toEqual([1_000_000, 2_000_000, 3_000_000]);
  });

  it('never shows a later frame after a short backward scrub', async () => {
    const { rerender } = renderAt(3n * S);
    await waitFor(() => expect(decoders).toHaveLength(1));

    rerender(
      <VideoLane source={source} topic="camera.cam0/h264" cursorNs={2n * S} startNs={S} endNs={3n * S} />,
    );
    expect(decoders).toHaveLength(2);
    expect(decoders[1].chunks.map((c) => c.timestamp)).toEqual([1_000_000, 2_000_000]);
  });
});
