import { vi } from 'vitest';
vi.mock('@/state/useFleet', () => ({
  useFleet: () => ({ rovers: [{ id: 'r1', name: 'Rover One', role: 'admin' }, { id: 'r2', name: 'Rover Two', role: 'admin' }], loading: false, refresh: vi.fn() }),
}));
vi.mock('@/views/RoverView/proxyModulesApi', () => ({
  getModuleReadme: vi.fn().mockResolvedValue('# Power'),
  deployModule: vi.fn().mockResolvedValue(undefined),
  checkModuleUpdates: vi.fn().mockResolvedValue({ ingested: [], latest: '1.5.0', autoUpdatedRovers: [] }),
  unpinRoverModule: vi.fn().mockResolvedValue(undefined),
  deleteModule: vi.fn().mockResolvedValue(undefined),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { ModuleDetailDrawer } from './ModuleDetailDrawer';
import type { MarketplaceModule } from '@/views/RoverView/proxyModulesApi';
import * as api from '@/views/RoverView/proxyModulesApi';

const mod: MarketplaceModule = {
  moduleId: 'power', displayName: 'Power Monitor', sourceRepoUrl: 'https://github.com/acme/power',
  repoVisibility: 'private', latestVersion: '1.5.0', hasReadme: true,
  rovers: [{ roverId: 'r1', desiredVersion: '1.4.2', autoUpdate: true, updateAvailable: true }],
};

beforeEach(() => vi.clearAllMocks());

describe('ModuleDetailDrawer', () => {
  it('loads readme and lists fleet rovers with install state', async () => {
    render(<ModuleDetailDrawer module={mod} open onClose={() => {}} onChanged={() => {}} />);
    await waitFor(() => expect(screen.getByText('Power')).toBeInTheDocument());
    expect(screen.getByText('Rover One')).toBeInTheDocument();
    expect(screen.getByText('Rover Two')).toBeInTheDocument();
  });

  it('deploys to an uninstalled rover', async () => {
    render(<ModuleDetailDrawer module={mod} open onClose={() => {}} onChanged={() => {}} />);
    await waitFor(() => screen.getByText('Rover Two'));
    fireEvent.click(screen.getByRole('button', { name: /install 1\.5\.0/i }));
    await waitFor(() => expect(api.deployModule).toHaveBeenCalledWith('power', [
      { roverId: 'r2', version: '1.5.0', autoUpdate: false, configToml: '' },
    ]));
  });

  it('removes the module from an installed rover', async () => {
    const onChanged = vi.fn();
    render(<ModuleDetailDrawer module={mod} open onClose={() => {}} onChanged={onChanged} />);
    await waitFor(() => screen.getByText('Rover One'));
    fireEvent.click(screen.getByTestId('remove-r1'));
    await waitFor(() => expect(api.unpinRoverModule).toHaveBeenCalledWith('r1', 'power'));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it('deletes the module from the registry after confirm', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    const onClose = vi.fn();
    render(<ModuleDetailDrawer module={mod} open onClose={onClose} onChanged={() => {}} />);
    await waitFor(() => screen.getByText('Rover One'));
    fireEvent.click(screen.getByTestId('delete-module'));
    await waitFor(() => expect(api.deleteModule).toHaveBeenCalledWith('power'));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('does not delete when confirm is dismissed', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    render(<ModuleDetailDrawer module={mod} open onClose={() => {}} onChanged={() => {}} />);
    await waitFor(() => screen.getByText('Rover One'));
    fireEvent.click(screen.getByTestId('delete-module'));
    expect(api.deleteModule).not.toHaveBeenCalled();
  });
});
