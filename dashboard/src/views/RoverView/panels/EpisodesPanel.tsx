// dashboard/src/views/RoverView/panels/EpisodesPanel.tsx
//
// Episodes tab: lists recorded episodes from the agent's local HTTP API,
// offers in-dashboard playback, per-episode download and a confirmed delete.
// Off-local sessions get a hint line: episodes live on the rover's local network.
import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { Panel } from '@/ui/primitives/Panel';
import { Button } from '@/ui/primitives/Button';
import { Chip } from '@/ui/primitives/Chip';
import { Dialog } from '@/ui/primitives/Dialog';
import { useMe } from '../../../state/mode';
import {
  listEpisodes,
  deleteEpisode,
  episodeDownloadUrl,
  type EpisodeMeta,
} from '../episodesApi';
import styles from './EpisodesPanel.module.css';

function formatDuration(s: number): string {
  const total = Math.max(0, Math.floor(s));
  const m = Math.floor(total / 60);
  const sec = total % 60;
  return `${m}:${String(sec).padStart(2, '0')}`;
}

function formatSize(bytes: number): string {
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatStart(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

type Outcome = { label: string; tone: 'default' | 'caution' | 'fault' };

function outcome(ep: EpisodeMeta): Outcome {
  if (ep.crashed) return { label: 'crashed', tone: 'fault' };
  if (ep.success === false) return { label: 'failed', tone: 'caution' };
  if (ep.success === true) return { label: 'success', tone: 'default' };
  return { label: 'N/A', tone: 'caution' };
}

export function EpisodesPanel() {
  const me = useMe();
  const isLocal = me?.mode === 'local';
  const { id: roverId } = useParams();

  const [episodes, setEpisodes] = useState<EpisodeMeta[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<EpisodeMeta | null>(null);

  async function refresh() {
    try {
      setEpisodes(await listEpisodes());
      setError(null);
    } catch (e) {
      setError(String((e as Error).message ?? e));
    }
  }

  useEffect(() => {
    if (isLocal) void refresh();
  }, [isLocal]);

  async function confirmDelete() {
    if (!pendingDelete) return;
    const id = pendingDelete.episode_id;
    setPendingDelete(null);
    try {
      await deleteEpisode(id);
      await refresh();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    }
  }

  if (me && !isLocal) {
    return (
      <Panel title="EPISODES">
        <p className={styles.hint}>
          Episodes are recorded and managed on the rover's local network.
        </p>
      </Panel>
    );
  }

  return (
    <div className={styles.layout} data-testid="panel-episodes">
      <Panel
        title="EPISODES"
        note={`${episodes.length} recorded`}
        action={<Button size="sm" onClick={() => void refresh()}>Refresh</Button>}
      >
        {error !== null && (
          <div className={styles.error} role="alert">{error}</div>
        )}
        {episodes.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyTitle}>No episodes yet</div>
            <div className={styles.emptyHint}>
              Start one from the record control in the teleop console.
            </div>
          </div>
        ) : (
          <ul className={styles.list}>
            {episodes.map((ep) => {
              const oc = outcome(ep);
              return (
                <li key={ep.episode_id} className={styles.row}>
                  <div className={styles.main}>
                    <div className={styles.label}>{ep.task_label || ep.episode_id}</div>
                    <div className={styles.meta}>
                      <span>{formatStart(ep.start)}</span>
                      <span className={styles.dot} aria-hidden>·</span>
                      <span>{formatDuration(ep.duration_s)}</span>
                      <span className={styles.dot} aria-hidden>·</span>
                      <span>{formatSize(ep.bytes)}</span>
                    </div>
                  </div>
                  <div className={styles.right}>
                    <Chip tone={oc.tone}>{oc.label}</Chip>
                    <Link
                      className={styles.download}
                      to={`/rover/${roverId}/episodes/${ep.episode_id}`}
                    >
                      Play
                    </Link>
                    <a
                      className={styles.download}
                      href={episodeDownloadUrl(ep.episode_id)}
                      download
                    >
                      Download
                    </a>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setPendingDelete(ep)}
                    >
                      Delete
                    </Button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>

      <Dialog
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title="Delete episode"
      >
        <p className={styles.confirmText}>
          Delete {pendingDelete?.task_label || pendingDelete?.episode_id}? This removes the MCAP
          file and its sidecar from the rover.
        </p>
        <div className={styles.confirmActions}>
          <Button onClick={() => setPendingDelete(null)}>Cancel</Button>
          <Button variant="danger" onClick={() => void confirmDelete()}>Confirm</Button>
        </div>
      </Dialog>
    </div>
  );
}
