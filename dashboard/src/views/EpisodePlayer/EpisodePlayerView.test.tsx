import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { EpisodePlayerView } from './EpisodePlayerView';

vi.mock('../../state/mode', () => ({ useMe: () => ({ mode: 'local' }) }));

const openMock = vi.fn();
vi.mock('../../lib/episode/episodeSource', () => ({
  EpisodeSource: { open: (...a: unknown[]) => openMock(...a) },
}));

vi.mock('../RoverView/episodesApi', () => ({
  listEpisodes: async () => [{
    format_version: 1, episode_id: 'ep-1', platform_id: 'waypoint-rover',
    rover_id: 'r1', task_label: 'dig test', start: '2026-07-30T10:00:00Z',
    end: '2026-07-30T10:01:00Z', duration_s: 60, success: null, notes: '',
    crashed: true, bytes: 1024, video_frames_dropped: 2, streams: [],
  }],
  episodeDownloadUrl: (id: string) => `/api/episodes/${id}/download`,
}));

function renderPlayer() {
  return render(
    <MemoryRouter initialEntries={['/rover/r1/episodes/ep-1']}>
      <Routes>
        <Route path="/rover/:id/episodes/:episodeId" element={<EpisodePlayerView />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('EpisodePlayerView', () => {
  beforeEach(() => {
    openMock.mockReset();
    openMock.mockResolvedValue({
      channels: () => [
        { topic: 'module.drill.sensor.state', schemaName: 'waypoint.v1.SensorReadings', count: 3, rateHz: 10, kind: 'plot' },
        { topic: 'module.drill.drill.state', schemaName: '', count: 2, rateHz: 20, kind: 'schemaless' },
      ],
      timeRange: () => ({ startNs: 0n, endNs: 1_000_000_000n }),
      series: async () => new Map(),
      videoAUs: async () => [],
    });
  });

  it('shows sidecar metadata and a crashed banner', async () => {
    renderPlayer();
    expect(await screen.findByText('dig test')).toBeInTheDocument();
    expect(await screen.findByText(/partial/i)).toBeInTheDocument();
  });

  it('lists plot channels as toggleable and schemaless as not decodable', async () => {
    renderPlayer();
    // Scoped to the sidebar: a visible plot topic also titles its lane.
    const sidebar = screen.getByRole('complementary');
    expect(await within(sidebar).findByText('module.drill.sensor.state')).toBeInTheDocument();
    expect(await within(sidebar).findByText(/not decodable/i)).toBeInTheDocument();
  });

  it('shows a closed state when the episode is gone', async () => {
    const { EpisodeGoneError } = await import('../../lib/episode/rangeReader');
    openMock.mockRejectedValue(new EpisodeGoneError());
    renderPlayer();
    expect(await screen.findByText(/no longer available/i)).toBeInTheDocument();
  });
});
