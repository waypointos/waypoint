import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ModuleTabFrame } from './ModuleTabFrame';

const mockUseMe = vi.fn();
vi.mock('@/state/mode', () => ({ useMe: () => mockUseMe() }));

const mockListDesired = vi.fn();
const mockSetDesired = vi.fn();
vi.mock('./proxyModulesApi', () => ({
  listRoverDesired: (...a: unknown[]) => mockListDesired(...a),
  setRoverDesired: (...a: unknown[]) => mockSetDesired(...a),
}));

beforeEach(() => {
  mockUseMe.mockReset();
  mockListDesired.mockReset();
  mockSetDesired.mockReset();
  mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: true });
  mockListDesired.mockResolvedValue([]);
  mockSetDesired.mockResolvedValue(undefined);
});

function renderFrame() {
  return render(
    <ModuleTabFrame moduleId="umr" roverId="rover-dev">
      <p>panel content</p>
    </ModuleTabFrame>,
  );
}

describe('ModuleTabFrame', () => {
  it('renders the module content untouched', () => {
    renderFrame();
    expect(screen.getByText('panel content')).toBeInTheDocument();
  });

  it('offers the gear to a proxy admin and saves the edited config', async () => {
    mockListDesired.mockResolvedValue([
      { moduleId: 'umr', version: '0.4.0', configToml: '', updatedAt: '2026-07-27T00:00:00Z' },
    ]);
    renderFrame();

    fireEvent.click(screen.getByTestId('configure-umr'));
    const box = await screen.findByLabelText('Module config TOML');
    fireEvent.change(box, { target: { value: 'password = "pw"\n' } });
    fireEvent.click(screen.getByTestId('save-module-config'));

    await waitFor(() =>
      expect(mockSetDesired).toHaveBeenCalledWith(
        'rover-dev',
        'umr',
        '0.4.0',
        '[modules_config.umr]\npassword = "pw"\n',
      ),
    );
  });

  it('hides the gear from a non-admin', () => {
    mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: false });
    renderFrame();
    expect(screen.queryByTestId('configure-umr')).toBeNull();
  });

  // Local mode has no config-update endpoint on the agent, so the gear would
  // only lead to a 404.
  it('hides the gear in local mode', () => {
    mockUseMe.mockReturnValue({ mode: 'local', isAdmin: true });
    renderFrame();
    expect(screen.queryByTestId('configure-umr')).toBeNull();
  });

  it('refuses to open with no desired row, which a save would unpin', async () => {
    mockListDesired.mockResolvedValue([]);
    renderFrame();
    fireEvent.click(screen.getByTestId('configure-umr'));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('not enabled on this rover'));
    expect(screen.queryByLabelText('Module config TOML')).toBeNull();
    expect(mockSetDesired).not.toHaveBeenCalled();
  });

  it('surfaces a fetch failure instead of opening an empty form', async () => {
    mockListDesired.mockRejectedValue(new Error('403: forbidden'));
    renderFrame();
    fireEvent.click(screen.getByTestId('configure-umr'));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('403: forbidden'));
    expect(screen.queryByLabelText('Module config TOML')).toBeNull();
  });
});
