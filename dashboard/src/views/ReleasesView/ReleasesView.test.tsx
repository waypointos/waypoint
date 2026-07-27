import { vi } from 'vitest';
vi.mock('./releasesApi', () => ({
  listReleasesFleet: vi.fn().mockResolvedValue([
    { roverId: 'r1', name: 'Rover One', currentVersion: '0.5.0', channel: 'prod', latestVersion: '0.6.0',
      updateAvailable: true, swuUrl: 'https://dl/x.swu', swuSha256: 'abc', releaseNotesMd: '## notes', releaseHtmlUrl: 'h' },
  ]),
  checkImageUpdates: vi.fn().mockResolvedValue(0),
  registerImageSource: vi.fn(),
  applyImage: vi.fn().mockResolvedValue(undefined),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ReleasesView } from './ReleasesView';
import * as api from './releasesApi';

const renderView = () =>
  render(<MemoryRouter initialEntries={['/releases']}><ReleasesView /></MemoryRouter>);

beforeEach(() => vi.clearAllMocks());

describe('ReleasesView', () => {
  it('lists rovers with current and latest version', async () => {
    renderView();
    await waitFor(() => expect(screen.getByText('Rover One')).toBeInTheDocument());
    expect(screen.getByText('0.5.0')).toBeInTheDocument();
    expect(screen.getByText('0.6.0')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /check for updates/i })).toBeInTheDocument();
  });

  it('Review opens the version-diff modal and applies', async () => {
    renderView();
    await waitFor(() => screen.getByText('Rover One'));
    fireEvent.click(screen.getByRole('button', { name: /review/i }));
    await waitFor(() => expect(screen.getByText('notes')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /apply & reboot/i }));
    await waitFor(() => expect(api.applyImage).toHaveBeenCalledWith('r1', 'https://dl/x.swu', 'abc', '0.6.0'));
  });
});
