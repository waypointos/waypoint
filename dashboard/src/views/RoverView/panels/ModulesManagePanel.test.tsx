// Mocks must be declared before any imports that transitively consume them.
import { vi } from 'vitest';

// useMe is overridden per-suite via mockReturnValue; start with a safe default.
vi.mock('@/state/mode', () => ({ useMe: vi.fn(() => ({ mode: 'local', isAdmin: true })) }));
vi.mock('@/state/session', () => ({ getToken: () => '' }));
vi.mock('@/state/useTelemetry', () => ({ useTelemetry: vi.fn(() => null) }));
vi.mock('../modulesApi', () => ({
  listModules: vi.fn(),
  stageModule: vi.fn(),
  confirmModule: vi.fn(),
  uninstallModule: vi.fn(),
}));
vi.mock('../proxyModulesApi', () => ({
  listRegistered: vi.fn(),
  listRoverDesired: vi.fn(),
  setRoverDesired: vi.fn(),
  unpinRoverModule: vi.fn(),
  deleteModule: vi.fn(),
  registerModule: vi.fn(),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ModulesManagePanel } from './ModulesManagePanel';
import * as api from '../modulesApi';
import * as proxyApi from '../proxyModulesApi';
import * as modeState from '@/state/mode';

const mockUseMe = vi.mocked(modeState.useMe);
const mockList = vi.mocked(api.listModules);
const mockStage = vi.mocked(api.stageModule);
const mockConfirm = vi.mocked(api.confirmModule);
const mockUninstall = vi.mocked(api.uninstallModule);
const mockListRegistered = vi.mocked(proxyApi.listRegistered);
const mockListDesired = vi.mocked(proxyApi.listRoverDesired);
const mockSetDesired = vi.mocked(proxyApi.setRoverDesired);
const mockUnpin = vi.mocked(proxyApi.unpinRoverModule);
const mockDeleteModule = vi.mocked(proxyApi.deleteModule);
const mockRegisterModule = vi.mocked(proxyApi.registerModule);

function renderPanel(roverId = 'rover-dev') {
  return render(<ModulesManagePanel roverId={roverId} />);
}

beforeEach(() => {
  vi.clearAllMocks();
  // Local API defaults
  mockList.mockResolvedValue([]);
  mockStage.mockResolvedValue({
    stageId: 'stage-1',
    id: 'power-monitor',
    version: '0.1.0',
    label: 'Power Monitor',
    signerSan: 'mailto:ops@example.com',
    uiKind: 'static',
    tabId: 'm-pm',
    permissions: ['sys.power'],
    devices: ['/dev/i2c-1'],
    pinned: false,
  });
  mockConfirm.mockResolvedValue(undefined);
  mockUninstall.mockResolvedValue(undefined);
  // Proxy API defaults
  mockListRegistered.mockResolvedValue([]);
  mockListDesired.mockResolvedValue([]);
  mockSetDesired.mockResolvedValue(undefined);
  mockRegisterModule.mockResolvedValue(undefined);
});

// ── LOCAL mode ──────────────────────────────────────────────────────────────

describe('ModulesManagePanel — local mode', () => {
  beforeEach(() => {
    mockUseMe.mockReturnValue({ mode: 'local', isAdmin: true });
  });

  it('calls listModules on mount', async () => {
    renderPanel();
    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(1));
  });

  it('renders the installed list with origin chips', async () => {
    mockList.mockResolvedValue([
      { id: 'power-monitor', label: 'Power Monitor', version: '0.1.0', origin: 'local' },
      { id: 'fleet-cam', label: 'Fleet Cam', version: '0.2.0', origin: 'proxy' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByText('power-monitor')).toBeInTheDocument());
    expect(screen.getByText('fleet-cam')).toBeInTheDocument();
    const chips = screen.getAllByText(/local|proxy/i);
    expect(chips.length).toBeGreaterThanOrEqual(2);
  });

  it('shows confirm card with signer SAN and permissions after verify', async () => {
    renderPanel();
    await waitFor(() => expect(mockList).toHaveBeenCalled());

    const rawInput = screen.getByLabelText(/module image/i);
    const bundleInput = screen.getByLabelText(/cosign bundle/i);
    const rawFile = new File(['raw'], 'pm.raw', { type: 'application/octet-stream' });
    const bundleFile = new File(['{}'], 'pm.bundle', { type: 'application/json' });
    Object.defineProperty(rawInput, 'files', { value: [rawFile], configurable: true });
    Object.defineProperty(bundleInput, 'files', { value: [bundleFile], configurable: true });

    fireEvent.click(screen.getByRole('button', { name: /verify/i }));

    await waitFor(() => expect(screen.getByTestId('confirm-card')).toBeInTheDocument());
    expect(screen.getByTestId('signer-san').textContent).toBe('mailto:ops@example.com');
    expect(screen.getByText('sys.power')).toBeInTheDocument();
  });

  it('calls confirmModule and refreshes the list after Trust & install', async () => {
    mockList.mockResolvedValueOnce([]).mockResolvedValueOnce([
      { id: 'power-monitor', label: 'Power Monitor', version: '0.1.0', origin: 'local' },
    ]);
    renderPanel();
    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(1));

    const rawInput = screen.getByLabelText(/module image/i);
    const bundleInput = screen.getByLabelText(/cosign bundle/i);
    Object.defineProperty(rawInput, 'files', { value: [new File(['raw'], 'pm.raw')], configurable: true });
    Object.defineProperty(bundleInput, 'files', { value: [new File(['{}'], 'pm.bundle')], configurable: true });

    fireEvent.click(screen.getByRole('button', { name: /verify/i }));
    await waitFor(() => expect(screen.getByTestId('confirm-card')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /trust & install/i }));
    await waitFor(() => expect(mockConfirm).toHaveBeenCalledWith('stage-1', ''));
    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(2));
    expect(screen.queryByTestId('confirm-card')).toBeNull();
    await waitFor(() => expect(screen.getByText('power-monitor')).toBeInTheDocument());
  });

  it('renders stage errors verbatim in the error region', async () => {
    mockStage.mockRejectedValue(new Error('signature verification failed: chain not trusted'));
    renderPanel();
    await waitFor(() => expect(mockList).toHaveBeenCalled());

    const rawInput = screen.getByLabelText(/module image/i);
    const bundleInput = screen.getByLabelText(/cosign bundle/i);
    Object.defineProperty(rawInput, 'files', { value: [new File(['r'], 'a.raw')], configurable: true });
    Object.defineProperty(bundleInput, 'files', { value: [new File(['{}'], 'a.bundle')], configurable: true });

    fireEvent.click(screen.getByRole('button', { name: /verify/i }));
    await waitFor(() =>
      expect(screen.getByTestId('error-region').textContent).toContain(
        'signature verification failed: chain not trusted',
      ),
    );
  });

  it('shows uninstall only for local-origin rows, not proxy-origin rows', async () => {
    mockList.mockResolvedValue([
      { id: 'local-mod', label: 'Local Mod', version: '0.1.0', origin: 'local' },
      { id: 'proxy-mod', label: 'Proxy Mod', version: '0.2.0', origin: 'proxy' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByText('local-mod')).toBeInTheDocument());

    const uninstallButtons = screen.getAllByRole('button', { name: /uninstall/i });
    expect(uninstallButtons).toHaveLength(1);
    expect(screen.getByText(/managed by proxy/i)).toBeInTheDocument();
  });

  it('calls uninstallModule with the module id', async () => {
    mockList.mockResolvedValue([
      { id: 'power-monitor', label: 'Power Monitor', version: '0.1.0', origin: 'local' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByText('power-monitor')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /uninstall/i }));
    await waitFor(() => expect(mockUninstall).toHaveBeenCalledWith('power-monitor'));
  });
});

// ── PROXY mode ──────────────────────────────────────────────────────────────

describe('ModulesManagePanel — proxy mode', () => {
  beforeEach(() => {
    mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: true });
  });

  it('does NOT call listModules (local /api/modules) on mount — crash regression guard', async () => {
    renderPanel();
    // Give the component a tick to settle any async mount effects.
    await waitFor(() => expect(mockListRegistered).toHaveBeenCalled());
    expect(mockList).not.toHaveBeenCalled();
  });

  it('calls listRegistered and listRoverDesired on mount', async () => {
    renderPanel();
    await waitFor(() => {
      expect(mockListRegistered).toHaveBeenCalledTimes(1);
      expect(mockListDesired).toHaveBeenCalledWith('rover-dev');
    });
  });

  it('renders the registry table with module rows', async () => {
    mockListRegistered.mockResolvedValue([
      {
        moduleId: 'power-monitor',
        displayName: 'Power Monitor',
        sourceRepoUrl: 'https://github.com/example/power-monitor',
        versions: [{ version: '0.2.0', ingestedAt: '2026-05-01T00:00:00Z', configFields: [] }],
      },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByText('power-monitor')).toBeInTheDocument());
    expect(screen.getByText('Power Monitor')).toBeInTheDocument();
    expect(screen.getByText('0.2.0')).toBeInTheDocument();
  });

  it('shows desired version chip when rover has a desired entry', async () => {
    mockListRegistered.mockResolvedValue([
      {
        moduleId: 'power-monitor',
        displayName: 'Power Monitor',
        sourceRepoUrl: 'https://github.com/example/power-monitor',
        versions: [{ version: '0.2.0', ingestedAt: '2026-05-01T00:00:00Z', configFields: [] }],
      },
    ]);
    mockListDesired.mockResolvedValue([
      { moduleId: 'power-monitor', version: '0.1.0', configToml: '', updatedAt: '2026-05-20T00:00:00Z' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByText('0.1.0')).toBeInTheDocument());
  });

  it('Enable collects config first, then sets desired at the latest version', async () => {
    mockListRegistered.mockResolvedValue([
      {
        moduleId: 'power-monitor',
        displayName: 'Power Monitor',
        sourceRepoUrl: 'https://github.com/example/power-monitor',
        versions: [{ version: '0.2.0', ingestedAt: '2026-05-01T00:00:00Z', configFields: [] }],
      },
    ]);
    mockSetDesired.mockResolvedValue(undefined);
    renderPanel();
    await waitFor(() => expect(screen.getByTestId('enable-power-monitor')).toBeInTheDocument());

    fireEvent.click(screen.getByTestId('enable-power-monitor'));
    const box = await screen.findByLabelText('Module config TOML');
    expect(mockSetDesired).not.toHaveBeenCalled();

    fireEvent.change(box, { target: { value: 'shunt_ohms = 0.01\n' } });
    fireEvent.click(screen.getByTestId('save-module-config'));
    await waitFor(() =>
      expect(mockSetDesired).toHaveBeenCalledWith(
        'rover-dev',
        'power-monitor',
        '0.2.0',
        '[modules_config.power-monitor]\nshunt_ohms = 0.01\n',
      ),
    );
  });

  it('gear on an enabled module edits its config, keeping the pinned version', async () => {
    mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: true } as never);
    mockListRegistered.mockResolvedValue([
      {
        moduleId: 'umr',
        displayName: 'Connectivity',
        sourceRepoUrl: 'https://github.com/waypointos/waypoint-umr',
        versions: [{ version: '0.4.0', ingestedAt: '2026-07-27T00:00:00Z', configFields: [] }],
      },
    ]);
    mockListDesired.mockResolvedValue([
      {
        moduleId: 'umr',
        version: '0.4.0',
        configToml: '[modules_config.umr]\nhost = "https://192.168.105.1"\n',
        updatedAt: '2026-07-27T00:00:00Z',
      },
    ]);
    mockSetDesired.mockResolvedValue(undefined);
    renderPanel();
    await waitFor(() => expect(screen.getByTestId('configure-umr')).toBeInTheDocument());

    fireEvent.click(screen.getByTestId('configure-umr'));
    // The stored table wrapper is the agent's concern; the operator sees flat keys.
    const box = (await screen.findByLabelText('Module config TOML')) as HTMLTextAreaElement;
    expect(box.value).toBe('host = "https://192.168.105.1"\n');

    fireEvent.change(box, { target: { value: 'host = "https://10.0.0.1"\npassword = "pw"\n' } });
    fireEvent.click(screen.getByTestId('save-module-config'));
    await waitFor(() =>
      expect(mockSetDesired).toHaveBeenCalledWith(
        'rover-dev',
        'umr',
        '0.4.0',
        '[modules_config.umr]\nhost = "https://10.0.0.1"\npassword = "pw"\n',
      ),
    );
  });

  it('Disable button unpins an enabled module from the rover', async () => {
    mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: true } as never);
    mockListRegistered.mockResolvedValue([
      {
        moduleId: 'power-monitor',
        displayName: 'Power Monitor',
        sourceRepoUrl: 'https://github.com/example/power-monitor',
        versions: [{ version: '0.2.0', ingestedAt: '2026-05-20T00:00:00Z', configFields: [] }],
      },
    ]);
    mockListDesired.mockResolvedValue([
      { moduleId: 'power-monitor', version: '0.2.0', configToml: '', updatedAt: '2026-05-20T00:00:00Z' },
    ]);
    mockUnpin.mockResolvedValue(undefined);
    renderPanel();
    await waitFor(() => expect(screen.getByTestId('disable-power-monitor')).toBeInTheDocument());
    expect(screen.queryByTestId('enable-power-monitor')).toBeNull();
    fireEvent.click(screen.getByTestId('disable-power-monitor'));
    await waitFor(() => expect(mockUnpin).toHaveBeenCalledWith('rover-dev', 'power-monitor'));
  });

  it('Remove deletes the module from the registry after confirm', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: true } as never);
    mockListRegistered.mockResolvedValue([
      {
        moduleId: 'power-monitor',
        displayName: 'Power Monitor',
        sourceRepoUrl: 'https://github.com/example/power-monitor',
        versions: [{ version: '0.2.0', ingestedAt: '2026-05-20T00:00:00Z', configFields: [] }],
      },
    ]);
    mockListDesired.mockResolvedValue([]);
    mockDeleteModule.mockResolvedValue(undefined);
    renderPanel();
    await waitFor(() => expect(screen.getByTestId('remove-power-monitor')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('remove-power-monitor'));
    await waitFor(() => expect(mockDeleteModule).toHaveBeenCalledWith('power-monitor'));
    // refreshProxy is called after enable
    expect(mockListRegistered).toHaveBeenCalledTimes(2);
  });

  it('renders the register form', async () => {
    renderPanel();
    await waitFor(() => expect(mockListRegistered).toHaveBeenCalled());
    expect(screen.getByLabelText(/source repo url/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/module id/i)).toBeNull();
    expect(screen.getByTestId('register-btn')).toBeInTheDocument();
  });

  it('Register button calls registerModule then refreshes', async () => {
    renderPanel();
    await waitFor(() => expect(mockListRegistered).toHaveBeenCalled());

    fireEvent.change(screen.getByLabelText(/source repo url/i), {
      target: { value: 'https://github.com/example/my-mod' },
    });
    fireEvent.click(screen.getByTestId('register-btn'));

    await waitFor(() =>
      expect(mockRegisterModule).toHaveBeenCalledWith('https://github.com/example/my-mod'),
    );
    expect(mockListRegistered).toHaveBeenCalledTimes(2);
  });

  it('shows error when listRegistered fails', async () => {
    mockListRegistered.mockRejectedValue(new Error('network error'));
    renderPanel();
    await waitFor(() =>
      expect(screen.getByTestId('error-region').textContent).toContain('network error'),
    );
  });
});
