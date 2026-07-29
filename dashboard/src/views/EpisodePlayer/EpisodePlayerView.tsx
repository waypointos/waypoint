// dashboard/src/views/EpisodePlayer/EpisodePlayerView.tsx
//
// Full-screen episode player: header from the sidecar, channel sidebar,
// video + plot lanes (mounted by later tasks), shared timeline transport.
import { useEffect, useMemo, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Chip } from '@/ui/primitives/Chip';
import { EpisodeSource } from '../../lib/episode/episodeSource';
import { RangeReadable, EpisodeGoneError } from '../../lib/episode/rangeReader';
import { newTransport, type Transport } from '../../lib/episode/transport';
import { listEpisodes, episodeDownloadUrl, type EpisodeMeta } from '../RoverView/episodesApi';
import { ChannelList } from './ChannelList';
import styles from './EpisodePlayerView.module.css';

type LoadState = 'loading' | 'ready' | 'gone' | 'error';

export function EpisodePlayerView() {
  const { id: roverId, episodeId } = useParams();
  const [state, setState] = useState<LoadState>('loading');
  const [source, setSource] = useState<EpisodeSource | null>(null);
  const [meta, setMeta] = useState<EpisodeMeta | null>(null);
  const [fallbackWarning, setFallbackWarning] = useState(false);
  const [visible, setVisible] = useState<string[] | null>(null);
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
      </main>
      <footer className={styles.timeline} data-testid="timeline">
        <span className={styles.cursorNs}>{transport.cursorNs.toString()}</span>
      </footer>
    </div>
  );
}
