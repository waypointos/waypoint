import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ModuleTabFrame } from './ModuleTabFrame';
import { RoverContextProvider, type RoverContextValue } from './RoverContext';
import { ModuleInfo, ModuleConfigField } from '../../../../protocol/gen/ts/messages/modules_pb';

const mockUseMe = vi.fn();
vi.mock('@/state/mode', () => ({ useMe: () => mockUseMe() }));

const mockListDesired = vi.fn();
const mockSetDesired = vi.fn();
vi.mock('./proxyModulesApi', () => ({
  listRoverDesired: (...a: unknown[]) => mockListDesired(...a),
  setRoverDesired: (...a: unknown[]) => mockSetDesired(...a),
}));

const umrSchema = [
  new ModuleConfigField({
    key: 'host', label: 'Router URL', type: 'url', defaultValue: 'https://192.168.105.1',
  }),
  new ModuleConfigField({
    key: 'password', label: 'Owner password', type: 'password', required: true,
    help: "The router's local owner password.",
  }),
  new ModuleConfigField({ key: 'poll_interval_s', label: 'Poll interval (s)', type: 'number' }),
];

function ctx(modules: ModuleInfo[]): RoverContextValue {
  return {
    id: 'rover-dev', role: 'admin', canControl: true,
    drive: null, power: null, sys: null, uplink: null, modules, image: null, motors: {},
    cameraNames: [], whepBase: '',
    roverAlerts: [], alertCounts: { info: 0, warning: 0, critical: 0 } as never,
    connection: { kind: 'offline' }, mode: 'unknown', recorder: null,
  };
}

beforeEach(() => {
  mockUseMe.mockReset();
  mockListDesired.mockReset();
  mockSetDesired.mockReset();
  mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: true });
  mockListDesired.mockResolvedValue([
    { moduleId: 'umr', version: '0.4.0', configToml: '', updatedAt: '2026-07-28T00:00:00Z' },
  ]);
  mockSetDesired.mockResolvedValue(undefined);
});

function renderFrame(modules: ModuleInfo[] = []) {
  return render(
    <RoverContextProvider value={ctx(modules)}>
      <ModuleTabFrame moduleId="umr" roverId="rover-dev">
        <p>panel content</p>
      </ModuleTabFrame>
    </RoverContextProvider>,
  );
}

const umrModule = (fields: ModuleConfigField[]) =>
  new ModuleInfo({ id: 'umr', label: 'Connectivity', configFields: fields });

describe('ModuleTabFrame', () => {
  it('renders the module content untouched', () => {
    renderFrame();
    expect(screen.getByText('panel content')).toBeInTheDocument();
  });

  it('renders declared fields instead of raw TOML, and saves them as TOML', async () => {
    mockListDesired.mockResolvedValue([
      {
        moduleId: 'umr', version: '0.4.0',
        configToml: '[modules_config.umr]\nhost = "https://192.168.105.1"\n',
        updatedAt: '2026-07-28T00:00:00Z',
      },
    ]);
    renderFrame([umrModule(umrSchema)]);

    fireEvent.click(screen.getByTestId('configure-umr'));
    const host = (await screen.findByLabelText('Router URL')) as HTMLInputElement;
    expect(host.value).toBe('https://192.168.105.1');
    expect(screen.queryByLabelText('Module config TOML')).toBeNull();
    expect(screen.getByText("The router's local owner password.")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Owner password'), { target: { value: 'pw' } });
    fireEvent.change(screen.getByLabelText('Poll interval (s)'), { target: { value: '5' } });
    fireEvent.click(screen.getByTestId('save-module-config'));

    await waitFor(() =>
      expect(mockSetDesired).toHaveBeenCalledWith(
        'rover-dev', 'umr', '0.4.0',
        '[modules_config.umr]\nhost = "https://192.168.105.1"\npassword = "pw"\npoll_interval_s = 5\n',
      ),
    );
  });

  it('masks a password field until it is revealed', async () => {
    renderFrame([umrModule(umrSchema)]);
    fireEvent.click(screen.getByTestId('configure-umr'));
    const pw = (await screen.findByLabelText('Owner password')) as HTMLInputElement;
    expect(pw.type).toBe('password');
    fireEvent.click(screen.getByLabelText('Show Owner password'));
    expect((screen.getByLabelText('Owner password') as HTMLInputElement).type).toBe('text');
  });

  it('blocks the save while a required field is empty', async () => {
    renderFrame([umrModule(umrSchema)]);
    fireEvent.click(screen.getByTestId('configure-umr'));
    await screen.findByLabelText('Router URL');
    expect(screen.getByTestId('save-module-config')).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Owner password'), { target: { value: 'pw' } });
    expect(screen.getByTestId('save-module-config')).toBeEnabled();
  });

  it('falls back to the TOML editor for a module that declares no schema', async () => {
    renderFrame([umrModule([])]);
    fireEvent.click(screen.getByTestId('configure-umr'));
    expect(await screen.findByLabelText('Module config TOML')).toBeInTheDocument();
    expect(screen.queryByLabelText('Router URL')).toBeNull();
  });

  it('hides the gear from a non-admin', () => {
    mockUseMe.mockReturnValue({ mode: 'proxy', isAdmin: false });
    renderFrame([umrModule(umrSchema)]);
    expect(screen.queryByTestId('configure-umr')).toBeNull();
  });

  // Local mode has no config-update endpoint on the agent, so the gear would
  // only lead to a 404.
  it('hides the gear in local mode', () => {
    mockUseMe.mockReturnValue({ mode: 'local', isAdmin: true });
    renderFrame([umrModule(umrSchema)]);
    expect(screen.queryByTestId('configure-umr')).toBeNull();
  });

  it('refuses to open with no desired row, which a save would unpin', async () => {
    mockListDesired.mockResolvedValue([]);
    renderFrame([umrModule(umrSchema)]);
    fireEvent.click(screen.getByTestId('configure-umr'));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('not enabled on this rover'));
    expect(screen.queryByLabelText('Router URL')).toBeNull();
    expect(mockSetDesired).not.toHaveBeenCalled();
  });

  it('surfaces a fetch failure instead of opening an empty form', async () => {
    mockListDesired.mockRejectedValue(new Error('403: forbidden'));
    renderFrame([umrModule(umrSchema)]);
    fireEvent.click(screen.getByTestId('configure-umr'));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('403: forbidden'));
    expect(screen.queryByLabelText('Router URL')).toBeNull();
  });
});
