// dashboard/src/views/EpisodePlayer/VideoLane.tsx
//
// Camera playback via WebCodecs. Seeks snap to the previous keyframe and
// decode forward; recording is keyframe-gated so every episode starts on one.
import { useEffect, useRef, useState } from 'react';
import type { EpisodeSource } from '../../lib/episode/episodeSource';
import { auInfo, codecStringFromSps } from '../../lib/episode/annexb';
import { keyframeBefore } from '../../lib/episode/transport';
import styles from './VideoLane.module.css';

type AU = { tNs: bigint; data: Uint8Array };
type Props = {
  source: EpisodeSource;
  topic: string;
  cursorNs: bigint;
  startNs: bigint;
  endNs: bigint;
};

export function VideoLane({ source, topic, cursorNs, startNs, endNs }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const ausRef = useRef<AU[]>([]);
  const keyTimesRef = useRef<bigint[]>([]);
  const runStartRef = useRef<bigint | null>(null);
  const lastFedRef = useRef<bigint | null>(null);
  const decoderRef = useRef<VideoDecoder | null>(null);
  const [status, setStatus] = useState<'loading' | 'ok' | 'unsupported'>(
    typeof VideoDecoder === 'undefined' ? 'unsupported' : 'loading',
  );

  useEffect(() => {
    if (typeof VideoDecoder === 'undefined') return;
    let cancelled = false;
    void source.videoAUs(topic, startNs, endNs).then((aus) => {
      if (cancelled) return;
      ausRef.current = aus;
      keyTimesRef.current = aus.filter((a) => auInfo(a.data).key).map((a) => a.tNs);
      setStatus('ok');
    });
    return () => { cancelled = true; };
  }, [source, topic, startNs, endNs]);

  useEffect(() => {
    if (status !== 'ok') return;
    const key = keyframeBefore(keyTimesRef.current, cursorNs);
    if (key === null) return;
    try {
      const lastFed = lastFedRef.current;
      // A different keyframe run, or any move back before what was already fed,
      // needs a fresh decoder: a decoder cannot replay frames it has passed.
      const reset = runStartRef.current !== key || lastFed === null || cursorNs < lastFed;
      if (reset) {
        decoderRef.current?.close();
        const first = ausRef.current.find((a) => a.tNs === key)!;
        const { sps } = auInfo(first.data);
        if (!sps) throw new Error('keyframe without sps');
        const decoder = new VideoDecoder({
          output: (frame) => {
            const canvas = canvasRef.current;
            const ctx = canvas?.getContext('2d');
            if (canvas && ctx) {
              canvas.width = frame.displayWidth;
              canvas.height = frame.displayHeight;
              ctx.drawImage(frame, 0, 0);
            }
            frame.close();
          },
          error: () => setStatus('unsupported'),
        });
        // No description: Annex-B mode with in-band SPS/PPS.
        decoder.configure({ codec: codecStringFromSps(sps) });
        decoderRef.current = decoder;
        runStartRef.current = key;
        lastFedRef.current = null;
      }
      const active = decoderRef.current;
      if (!active) return;
      // From the keyframe after a reset, otherwise only what has not been fed.
      const from = reset ? key : lastFed! + 1n;
      for (const au of ausRef.current) {
        if (au.tNs < from || au.tNs > cursorNs) continue;
        active.decode(new EncodedVideoChunk({
          type: auInfo(au.data).key ? 'key' : 'delta',
          timestamp: Number(au.tNs / 1000n),
          data: au.data,
        }));
        lastFedRef.current = au.tNs;
      }
    } catch {
      setStatus('unsupported');
    }
  }, [cursorNs, status]);

  useEffect(() => () => decoderRef.current?.close(), []);

  if (status === 'unsupported') {
    return <div className={styles.notice}>video decoding unavailable in this browser</div>;
  }
  return <canvas ref={canvasRef} className={styles.video} data-testid="video-lane" />;
}
