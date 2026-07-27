import { vi } from 'vitest';
vi.mock('@/state/useFleet', () => ({ useFleet: () => ({ rovers: [], loading: false, refresh: vi.fn() }) }));
vi.mock('@/views/RoverView/proxyModulesApi', () => ({
  listMarketplace: vi.fn().mockResolvedValue([
    { moduleId: 'power', displayName: 'Power Monitor', sourceRepoUrl: 'https://github.com/acme/power',
      repoVisibility: 'public', latestVersion: '1.5.0', hasReadme: false, rovers: [] },
  ]),
  checkModuleUpdates: vi.fn(), getModuleReadme: vi.fn(), deployModule: vi.fn(), registerModule: vi.fn(),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ModulesView } from './ModulesView';

beforeEach(() => vi.clearAllMocks());

describe('ModulesView', () => {
  it('lists registered modules as cards', async () => {
    render(<MemoryRouter initialEntries={['/modules']}><ModulesView /></MemoryRouter>);
    await waitFor(() => expect(screen.getByText('Power Monitor')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: /register module/i })).toBeInTheDocument();
  });
});
