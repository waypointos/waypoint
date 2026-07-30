import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { EpisodesPanel } from './EpisodesPanel';

const listEpisodes = vi.fn();
const deleteEpisode = vi.fn().mockResolvedValue(undefined);
vi.mock('../episodesApi', () => ({
  listEpisodes: () => listEpisodes(),
  deleteEpisode: (id: string) => deleteEpisode(id),
  episodeDownloadUrl: (id: string) => `/api/episodes/${id}/download`,
}));
vi.mock('../../../state/mode', () => ({
  useMe: () => ({ mode: 'local' }),
}));

const EP = {
  format_version: 1, episode_id: 'ep-20260612T150000Z-ab12', platform_id: 'waypoint-rover',
  rover_id: 'dev', task_label: 'push the block', start: '2026-06-12T15:00:00Z',
  end: '2026-06-12T15:01:38Z', duration_s: 98.2, success: true, notes: '', crashed: false,
  bytes: 73400320, video_frames_dropped: 0,
  streams: [{ subject: 'telemetry.drive', message: 'waypoint.v1.DriveTelemetry', count: 4910 }],
};

// The panel links into the player route, so it needs a router with :id bound.
function renderPanel() {
  return render(
    <MemoryRouter initialEntries={['/rover/dev/episodes']}>
      <Routes>
        <Route path="/rover/:id/:tab" element={<EpisodesPanel />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('EpisodesPanel', () => {
  it('lists episodes with label, duration, size, and outcome', async () => {
    listEpisodes.mockResolvedValue([EP]);
    renderPanel();
    await waitFor(() => expect(screen.getByText('push the block')).toBeInTheDocument());
    expect(screen.getByText(/1:38/)).toBeInTheDocument();
    expect(screen.getByText(/70\.0 MB/)).toBeInTheDocument();
    expect(screen.getByText(/success/i)).toBeInTheDocument();
  });

  it('marks crashed episodes distinctly', async () => {
    listEpisodes.mockResolvedValue([{ ...EP, success: null, crashed: true }]);
    renderPanel();
    await waitFor(() => expect(screen.getByText(/crashed/i)).toBeInTheDocument());
  });

  it('deletes after confirmation', async () => {
    listEpisodes.mockResolvedValue([EP]);
    renderPanel();
    await waitFor(() => screen.getByText('push the block'));
    fireEvent.click(screen.getByRole('button', { name: /delete/i }));
    fireEvent.click(screen.getByRole('button', { name: /confirm/i }));
    await waitFor(() => expect(deleteEpisode).toHaveBeenCalledWith('ep-20260612T150000Z-ab12'));
  });

  it('shows an explanatory empty state', async () => {
    listEpisodes.mockResolvedValue([]);
    renderPanel();
    await waitFor(() => expect(screen.getByText(/no episodes/i)).toBeInTheDocument());
  });

  it('renders a Play link per episode on local sessions', async () => {
    listEpisodes.mockResolvedValue([EP]);
    renderPanel();
    expect(await screen.findByRole('link', { name: /play/i })).toHaveAttribute(
      'href',
      '/rover/dev/episodes/ep-20260612T150000Z-ab12',
    );
  });
});
