// dashboard/src/lib/episode/episodeSource.ts
//
// Indexed access to one episode .mcap. Opening reads only the summary
// (schemas, channels, chunk index, statistics); message reads pull just the
// chunks overlapping the requested window.
import { McapIndexedReader } from '@mcap/core';
import type { IReadable } from '@mcap/core';
import { decompress as zstdDecompress } from 'fzstd';
import { decodeBySchema, extractSeries, VIDEO_SCHEMA, SCHEMA_REGISTRY, type Sample } from './decoders';
import { pbFromBinary } from '../../state/protobuf';
import { CompressedVideo } from '../../../../protocol/gen/ts/foxglove/compressed_video_pb';

// The recorder writes ZSTD chunks. Decompression is pure JS on purpose:
// @mcap/support's wasm handlers need a Node Buffer and are not bundleable here.
const DECOMPRESS_HANDLERS = {
  zstd: (buffer: Uint8Array, decompressedSize: bigint) =>
    zstdDecompress(buffer, new Uint8Array(Number(decompressedSize))),
};

export type ChannelInfo = {
  topic: string;
  schemaName: string;
  count: number;
  rateHz: number | null;
  kind: 'plot' | 'video' | 'schemaless';
};

export class EpisodeSource {
  private constructor(
    private reader: McapIndexedReader,
    private infos: ChannelInfo[],
    private schemaByChannelId: Map<number, string>,
    private topicByChannelId: Map<number, string>,
  ) {}

  static async open(readable: IReadable): Promise<EpisodeSource> {
    const reader = await McapIndexedReader.Initialize({
      readable,
      decompressHandlers: DECOMPRESS_HANDLERS,
    });
    const stats = reader.statistics;
    const spanS = stats ? Number(stats.messageEndTime - stats.messageStartTime) / 1e9 : 0;
    const schemaByChannelId = new Map<number, string>();
    const topicByChannelId = new Map<number, string>();
    const infos: ChannelInfo[] = [];
    for (const ch of reader.channelsById.values()) {
      const schema = ch.schemaId === 0 ? '' : (reader.schemasById.get(ch.schemaId)?.name ?? '');
      schemaByChannelId.set(ch.id, schema);
      topicByChannelId.set(ch.id, ch.topic);
      const count = Number(stats?.channelMessageCounts.get(ch.id) ?? 0n);
      const known = schema !== '' && schema in SCHEMA_REGISTRY;
      infos.push({
        topic: ch.topic,
        schemaName: schema,
        count,
        // n messages span n-1 intervals, so rate divides intervals by the span.
        rateHz: spanS > 0 && count > 1 ? (count - 1) / spanS : null,
        kind: schema === VIDEO_SCHEMA ? 'video' : known ? 'plot' : 'schemaless',
      });
    }
    infos.sort((a, b) => a.topic.localeCompare(b.topic));
    return new EpisodeSource(reader, infos, schemaByChannelId, topicByChannelId);
  }

  channels(): ChannelInfo[] {
    return this.infos;
  }

  timeRange(): { startNs: bigint; endNs: bigint } {
    const s = this.reader.statistics;
    return { startNs: s?.messageStartTime ?? 0n, endNs: s?.messageEndTime ?? 0n };
  }

  async series(topics: string[], startNs: bigint, endNs: bigint): Promise<Map<string, Sample[]>> {
    const out = new Map<string, Sample[]>();
    for await (const msg of this.reader.readMessages({
      topics,
      startTime: startNs,
      endTime: endNs,
    })) {
      const topic = this.topicByChannelId.get(msg.channelId)!;
      const schema = this.schemaByChannelId.get(msg.channelId)!;
      const decoded = decodeBySchema(schema, msg.data);
      if (!decoded) continue;
      for (const { id, sample } of extractSeries(topic, schema, decoded, msg.logTime)) {
        const arr = out.get(id) ?? [];
        arr.push(sample);
        out.set(id, arr);
      }
    }
    return out;
  }

  async videoAUs(
    topic: string,
    startNs: bigint,
    endNs: bigint,
  ): Promise<Array<{ tNs: bigint; data: Uint8Array }>> {
    const out: Array<{ tNs: bigint; data: Uint8Array }> = [];
    for await (const msg of this.reader.readMessages({
      topics: [topic],
      startTime: startNs,
      endTime: endNs,
    })) {
      const frame = pbFromBinary<{ data: Uint8Array }>(CompressedVideo, msg.data);
      out.push({ tNs: msg.logTime, data: frame.data });
    }
    return out;
  }
}
