// dashboard/src/views/RoverView/episodesApi.ts
//
// Typed client for the agent's local episode HTTP API (list/download/delete).
import { apiUrl, check } from './modulesApi';

export type EpisodeStream = { subject: string; message: string; count: number };

export type EpisodeMeta = {
  format_version: number;
  episode_id: string;
  platform_id: string;
  rover_id: string;
  task_label: string;
  start: string;
  end: string;
  duration_s: number;
  success: boolean | null;
  notes: string;
  crashed: boolean;
  bytes: number;
  video_frames_dropped: number;
  streams: EpisodeStream[] | null;
};

export async function listEpisodes(): Promise<EpisodeMeta[]> {
  const r = await fetch(apiUrl('/api/episodes'), { credentials: 'include' });
  await check(r);
  return r.json();
}

export async function deleteEpisode(id: string): Promise<void> {
  const r = await fetch(apiUrl(`/api/episodes/${encodeURIComponent(id)}`), { method: 'DELETE', credentials: 'include' });
  await check(r);
}

export function episodeDownloadUrl(id: string): string {
  return apiUrl(`/api/episodes/${encodeURIComponent(id)}/download`);
}
