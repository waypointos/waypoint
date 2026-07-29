// dashboard/src/views/EpisodePlayer/EpisodePlayerView.tsx
//
// Full-screen episode player: header from the sidecar, channel sidebar,
// video + plot lanes, shared timeline transport.
import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Chip } from '@/ui/primitives/Chip';
import { Button } from '@/ui/primitives/Button';
import { EpisodeSource } from '../../lib/episode/episodeSource';
import { RangeReadable, EpisodeGoneError } from '../../lib/episode/rangeReader';
import {
  newTransport, tick, play, pause, setSpeed, seek, SPEEDS, type Transport,
} from '../../lib/episode/transport';
import type { Sample } from '../../lib/episode/decoders';
import { listEpisodes, episodeDownloadUrl, type EpisodeMeta } from '../RoverView/episodesApi';
import { ChannelList } from './ChannelList';
import { PlotLane } from './PlotLane';
import { TimelineScrubber } from './TimelineScrubber';
import styles from './EpisodePlayerView.module.css';

type LoadState = 'loading' | 'ready' | 'gone' | 'error';
type Sel = { startNs: bigint; endNs: bigint };

function elapsedS(tNs: bigint, startNs: bigint): string {
  return (Number(tNs - startNs) / 1e9).toFixed(2);
}

export function EpisodePlayerView() {
  const { id: roverId, episodeId } = useParams();
  const [state, setState] = useState<LoadState>('loading');
  const [source, setSource] = useState<EpisodeSource | null>(null);
  const [meta, setMeta] = useState<EpisodeMeta | null>(null);
  const [fallbackWarning, setFallbackWarning] = useState(false);
  const [visible, setVisible] = useState<string[] | null>(null);
  const [series, setSeries] = useState<Map<string, Sample[]>>(() => new Map());
  const [selection, setSelection] = useState<Sel | null>(null);
  const [transport, setTransport] = useState<Transport>(() => newTransport(0n));

  useEffect(() => {
    if (!episodeId) return;
    let cancelled = false;
    (async () => {
      try {
        const readable = new RangeReadable(episodeDownloadUrl(episodeId));
        const [src, all] = await Promise.all([EpisodeSource.open(readable), listEpisodes()]);
        if (cancelled) return;
        setSource(src);
        setMeta(all.find((e) => e.episode_id === episodeId) ?? null);
        setFallbackWarning(readable.usedFullFetchFallback());
        setTransport(newTransport(src.timeRange().startNs));
        setVisible(
          src.channels()
            .filter((c) => c.kind === 'plot')
            .filter((c) => c.topic.startsWith('module.') || c.topic === 'telemetry.motors')
            .map((c) => c.topic),
        );
        setState('ready');
      } catch (e) {
        if (cancelled) return;
        setState(e instanceof EpisodeGoneError ? 'gone' : 'error');
      }
    })();
    return () => { cancelled = true; };
  }, [episodeId]);

  const channels = useMemo(() => source?.channels() ?? [], [source]);
  const range = useMemo(
    () => source?.timeRange() ?? { startNs: 0n, endNs: 0n },
    [source],
  );

  // Load every visible topic over the whole episode; the source can window
  // reads, but a full load keeps scrubbing instant on episode-sized files.
  useEffect(() => {
    if (!source || visible === null) return;
    if (visible.length === 0) {
      setSeries(new Map());
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const loaded = await source.series(visible, range.startNs, range.endNs);
        if (!cancelled) setSeries(loaded);
      } catch (e) {
        if (cancelled) return;
        setState(e instanceof EpisodeGoneError ? 'gone' : 'error');
      }
    })();
    return () => { cancelled = true; };
  }, [source, visible, range.startNs, range.endNs]);

  // Series ids belonging to each visible topic, stable so lanes only redraw
  // when the data or the cursor actually changes.
  const laneIds = useMemo(() => {
    const m = new Map<string, string[]>();
    for (const topic of visible ?? []) {
      m.set(topic, [...series.keys()].filter((k) => k.startsWith(`${topic}.`)));
    }
    return m;
  }, [series, visible]);

  const lastFrameRef = useRef(0);
  useEffect(() => {
    if (!transport.playing) return;
    lastFrameRef.current = performance.now();
    let raf = 0;
    const frame = (now: number) => {
      const dtMs = now - lastFrameRef.current;
      lastFrameRef.current = now;
      setTransport((t) => tick(t, dtMs, range.endNs));
      raf = requestAnimationFrame(frame);
    };
    raf = requestAnimationFrame(frame);
    return () => cancelAnimationFrame(raf);
  }, [transport.playing, range.endNs]);

  if (state === 'gone') {
    return (
      <div className={styles.closed}>
        <p>This episode is no longer available on the rover.</p>
        <Link to={`/rover/${roverId}/episodes`}>Back to episodes</Link>
      </div>
    );
  }

  return (
    <div className={styles.layout} data-testid="episode-player">
      <header className={styles.header}>
        <div className={styles.title}>{meta?.task_label || episodeId}</div>
        <div className={styles.headerMeta}>
          {meta?.crashed && <Chip tone="fault">partial (crashed)</Chip>}
          {fallbackWarning && <Chip tone="caution">full download (no range support)</Chip>}
          {meta && <span>{meta.duration_s.toFixed(0)}s · {(meta.bytes / (1024 * 1024)).toFixed(1)} MB</span>}
          {meta != null && meta.video_frames_dropped > 0 && (
            <span>{meta.video_frames_dropped} video frames dropped</span>
          )}
        </div>
        <Link className={styles.back} to={`/rover/${roverId}/episodes`}>Close</Link>
      </header>
      <aside className={styles.sidebar}>
        <ChannelList
          channels={channels}
          visible={visible ?? []}
          onToggle={(topic) =>
            setVisible((v) => (v ?? []).includes(topic)
              ? (v ?? []).filter((t) => t !== topic)
              : [...(v ?? []), topic])
          }
        />
      </aside>
      <main className={styles.stage}>
        {state === 'loading' && <div className={styles.loading}>Loading episode…</div>}
        {state === 'error' && <div className={styles.loading} role="alert">Failed to open episode.</div>}
        {(visible ?? []).map((topic) => (
          <PlotLane
            key={topic}
            title={topic}
            series={series}
            ids={laneIds.get(topic) ?? []}
            startNs={range.startNs}
            endNs={range.endNs}
            cursorNs={transport.cursorNs}
          />
        ))}
      </main>
      <footer className={styles.timeline} data-testid="timeline">
        <Button
          size="sm"
          onClick={() => setTransport((t) => (t.playing ? pause(t) : play(t)))}
        >
          {transport.playing ? 'Pause' : 'Play'}
        </Button>
        <select
          className={styles.speed}
          aria-label="Playback speed"
          value={transport.speed}
          onChange={(e) => setTransport((t) => setSpeed(t, Number(e.target.value)))}
        >
          {SPEEDS.map((s) => <option key={s} value={s}>{s}x</option>)}
        </select>
        <TimelineScrubber
          startNs={range.startNs}
          endNs={range.endNs}
          cursorNs={transport.cursorNs}
          selection={selection}
          onSeek={(t) => setTransport((prev) => seek(prev, t, range.startNs, range.endNs))}
          onSelect={setSelection}
        />
        <span className={styles.clock}>
          {elapsedS(transport.cursorNs, range.startNs)} / {elapsedS(range.endNs, range.startNs)} s
        </span>
      </footer>
    </div>
  );
}
