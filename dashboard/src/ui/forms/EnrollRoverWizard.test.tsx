import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { EnrollRoverWizard } from './EnrollRoverWizard';

function noop() {}
function stubBus() {
  return { subscribe: () => () => {} };
}

describe('EnrollRoverWizard — step 01 (Identify)', () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  beforeEach(() => {
    fetchMock = vi.fn();
    vi.spyOn(globalThis, 'fetch').mockImplementation(fetchMock as unknown as typeof fetch);
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the Identify step with id and name fields and a Generate button', () => {
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    expect(screen.getByLabelText('Rover ID')).toBeInTheDocument();
    expect(screen.getByLabelText('Name')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /generate identity/i })).toBeDisabled();
  });

  it('enables Generate when both fields validate', () => {
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    expect(screen.getByRole('button', { name: /generate identity/i })).toBeEnabled();
  });

  it('shows an inline error for invalid rover ID', () => {
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'Bad ID!' } });
    fireEvent.blur(screen.getByLabelText('Rover ID'));
    expect(screen.getByText(/lowercase alphanumeric/i)).toBeInTheDocument();
  });

  it('POSTs id and name on submit and advances on 200', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ roverId: 'sim-02', identityToml: '[rover]\nid = "sim-02"\n' }),
    });
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));

    await waitFor(() => {
      expect(screen.getByText(/02 generate/i)).toBeInTheDocument();
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/rovers/enroll',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        body: JSON.stringify({ id: 'sim-02', name: 'Recon' }),
      }),
    );
  });

  it('surfaces server errors and stays on step 01', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 400,
      text: async () => 'rover id already exists',
    });
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));

    await waitFor(() => {
      expect(screen.getByText('rover id already exists')).toBeInTheDocument();
    });
    expect(screen.queryByText(/02 generate/i)).toBeNull();
  });

  it('resets form state when reopened after close', async () => {
    const { rerender } = render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });

    rerender(<EnrollRoverWizard open={false} onClose={noop} bus={stubBus()} />);
    rerender(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);

    expect(screen.getByLabelText('Rover ID')).toHaveValue('');
    expect(screen.getByLabelText('Name')).toHaveValue('');
  });
});

describe('EnrollRoverWizard — step 02 (Generate)', () => {
  beforeEach(() => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        roverId: 'sim-02',
        identityToml: '[rover]\nid   = "sim-02"\nname = "Recon"\n',
      }),
    });
    vi.spyOn(globalThis, 'fetch').mockImplementation(fetchMock as unknown as typeof fetch);
  });
  afterEach(() => { vi.restoreAllMocks(); });

  async function advanceToGenerate() {
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));
    await waitFor(() => screen.getByText(/02 generate/i));
  }

  it('renders the CodeViewer with the generated identity.toml', async () => {
    await advanceToGenerate();
    expect(screen.getByText('IDENTITY.TOML')).toBeInTheDocument();
    expect(screen.getByTestId('code-viewer-body').textContent).toContain('id   = "sim-02"');
  });

  it('copies TOML content to clipboard when Copy is clicked', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    await advanceToGenerate();
    fireEvent.click(screen.getByLabelText('Copy'));
    expect(writeText).toHaveBeenCalledWith(
      '[rover]\nid   = "sim-02"\nname = "Recon"\n',
    );
  });

  it('Back returns to step 01 with previous values preserved', async () => {
    await advanceToGenerate();
    fireEvent.click(screen.getByRole('button', { name: /^back$/i }));
    expect(screen.getByLabelText('Rover ID')).toHaveValue('sim-02');
    expect(screen.getByLabelText('Name')).toHaveValue('Recon');
  });

  it('Continue advances to step 03 (Flash & Drop)', async () => {
    await advanceToGenerate();
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    expect(screen.getByText(/03 flash & drop/i)).toBeInTheDocument();
  });
});

describe('EnrollRoverWizard — step 03 (Flash & Drop)', () => {
  beforeEach(() => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        roverId: 'sim-02',
        identityToml: '[rover]\nid = "sim-02"\n',
      }),
    });
    vi.spyOn(globalThis, 'fetch').mockImplementation(fetchMock as unknown as typeof fetch);
  });
  afterEach(() => { vi.restoreAllMocks(); });

  async function advanceToFlashDrop() {
    render(<EnrollRoverWizard open onClose={noop} bus={stubBus()} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));
    await waitFor(() => screen.getByText(/02 generate/i));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    await waitFor(() => screen.getByText(/03 flash & drop/i));
  }

  it('renders both Flash and Drop section headers and external links', async () => {
    await advanceToFlashDrop();
    expect(screen.getByText(/A · FLASH THE IMAGE/)).toBeInTheDocument();
    expect(screen.getByText(/B · DROP IDENTITY\.TOML/)).toBeInTheDocument();
    const imager = screen.getByRole('link', { name: /raspberry pi imager/i });
    expect(imager).toHaveAttribute('target', '_blank');
    expect(imager).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(screen.getByRole('link', { name: /latest release on github/i })).toBeInTheDocument();
  });

  it('renders a compact CodeViewer with no body but actions still work', async () => {
    await advanceToFlashDrop();
    expect(screen.getAllByText('IDENTITY.TOML').length).toBeGreaterThan(0);
    expect(screen.queryByTestId('code-viewer-body')).toBeNull();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    fireEvent.click(screen.getByLabelText('Copy'));
    expect(writeText).toHaveBeenCalled();
  });

  it('does not offer a modules.toml drop (modules install at runtime)', async () => {
    await advanceToFlashDrop();
    expect(screen.queryByText(/C · DROP MODULES\.TOML/)).toBeNull();
    expect(screen.queryByText('MODULES.TOML')).toBeNull();
  });

  it('identity.toml carries no [[cameras]] (agent auto-discovers at boot)', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    await advanceToFlashDrop();
    fireEvent.click(screen.getAllByLabelText('Copy')[0]);
    const copied = writeText.mock.calls.at(-1)?.[0] ?? '';
    expect(copied).not.toContain('[[cameras]]');
  });

  it('I have done this advances to step 04 (Boot & Verify)', async () => {
    await advanceToFlashDrop();
    fireEvent.click(screen.getByRole('button', { name: /i've done this/i }));
    expect(screen.getByText(/04 boot & verify/i)).toBeInTheDocument();
  });
});

describe('EnrollRoverWizard — step 04 (Boot & Verify)', () => {
  beforeEach(() => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        roverId: 'sim-02',
        identityToml: '[rover]\nid = "sim-02"\n',
      }),
    });
    vi.spyOn(globalThis, 'fetch').mockImplementation(fetchMock as unknown as typeof fetch);
  });
  afterEach(() => { vi.restoreAllMocks(); });

  type Subscribe = (subject: string, handler: (data: Uint8Array) => void) => () => void;
  type BusStub = {
    subscribe: ReturnType<typeof vi.fn<Subscribe>>;
    emit: (subject: string, data?: Uint8Array) => void;
  };
  function captureBus(): BusStub {
    const handlers = new Map<string, (d: Uint8Array) => void>();
    const subscribe = vi.fn<Subscribe>(
      (subject: string, h: (d: Uint8Array) => void) => {
        handlers.set(subject, h);
        return () => { handlers.delete(subject); };
      },
    );
    return {
      subscribe,
      emit: (subject, data = new Uint8Array()) => handlers.get(subject)?.(data),
    };
  }

  async function advanceToVerify(bus: BusStub) {
    render(<EnrollRoverWizard open onClose={noop} bus={{ subscribe: bus.subscribe }} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));
    await waitFor(() => screen.getByText(/02 generate/i));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    await waitFor(() => screen.getByText(/03 flash & drop/i));
    fireEvent.click(screen.getByRole('button', { name: /i've done this/i }));
  }

  it('subscribes to waypoint.<id>.infra.heartbeat on entry', async () => {
    const bus = captureBus();
    await advanceToVerify(bus);
    expect(bus.subscribe).toHaveBeenCalledWith(
      'waypoint.sim-02.infra.heartbeat',
      expect.any(Function),
    );
    expect(screen.getByText(/waiting for first heartbeat/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^done$/i })).toBeDisabled();
  });

  it('transitions to ONLINE on first heartbeat and enables Done', async () => {
    const bus = captureBus();
    await advanceToVerify(bus);
    bus.emit('waypoint.sim-02.infra.heartbeat');
    await waitFor(() => screen.getByText(/online · last seen now/i));
    expect(screen.getByRole('button', { name: /^done$/i })).toBeEnabled();
    expect(screen.getByText(/first heartbeat at/i)).toBeInTheDocument();
  });

  it('Done closes the drawer', async () => {
    const onClose = vi.fn();
    const bus = captureBus();
    render(<EnrollRoverWizard open onClose={onClose} bus={{ subscribe: bus.subscribe }} />);
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));
    await waitFor(() => screen.getByText(/02 generate/i));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    await waitFor(() => screen.getByText(/03 flash & drop/i));
    fireEvent.click(screen.getByRole('button', { name: /i've done this/i }));
    bus.emit('waypoint.sim-02.infra.heartbeat');
    await waitFor(() => screen.getByRole('button', { name: /^done$/i }));
    fireEvent.click(screen.getByRole('button', { name: /^done$/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('unsubscribes when the drawer closes mid-wait', async () => {
    const handlers = new Map<string, (d: Uint8Array) => void>();
    const unsub = vi.fn();
    const subscribe = vi.fn<Subscribe>(
      (subject: string, h: (d: Uint8Array) => void) => {
        handlers.set(subject, h);
        return unsub;
      },
    );
    const onClose = vi.fn();
    const { rerender } = render(
      <EnrollRoverWizard open onClose={onClose} bus={{ subscribe }} />,
    );
    fireEvent.change(screen.getByLabelText('Rover ID'), { target: { value: 'sim-02' } });
    fireEvent.change(screen.getByLabelText('Name'),     { target: { value: 'Recon' } });
    fireEvent.click(screen.getByRole('button', { name: /generate identity/i }));
    await waitFor(() => screen.getByText(/02 generate/i));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    await waitFor(() => screen.getByText(/03 flash & drop/i));
    fireEvent.click(screen.getByRole('button', { name: /i've done this/i }));
    rerender(<EnrollRoverWizard open={false} onClose={onClose} bus={{ subscribe }} />);
    expect(unsub).toHaveBeenCalled();
  });
});
