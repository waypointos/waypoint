import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ModuleCard } from './ModuleCard';
import type { MarketplaceModule } from '@/views/RoverView/proxyModulesApi';

const mod: MarketplaceModule = {
  moduleId: 'power', displayName: 'Power Monitor', sourceRepoUrl: 'https://github.com/acme/power',
  repoVisibility: 'private', latestVersion: '1.5.0', hasReadme: true,
  rovers: [{ roverId: 'r1', desiredVersion: '1.4.2', autoUpdate: true, updateAvailable: true }],
};

describe('ModuleCard', () => {
  it('renders name, repo, latest version and an update chip when a rover is behind', () => {
    render(<ModuleCard module={mod} onOpen={() => {}} />);
    expect(screen.getByText('Power Monitor')).toBeInTheDocument();
    expect(screen.getByText('1.5.0')).toBeInTheDocument();
    expect(screen.getByText(/update/i)).toBeInTheDocument();
  });

  it('calls onOpen when clicked', () => {
    const onOpen = vi.fn();
    render(<ModuleCard module={mod} onOpen={onOpen} />);
    fireEvent.click(screen.getByText('Power Monitor'));
    expect(onOpen).toHaveBeenCalledWith('power');
  });
});
