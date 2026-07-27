import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { ModulePanel } from './ModulePanel';

const busSubscribe = vi.fn((_subject: string, _cb: (b: Uint8Array) => void) => () => {});
const busPublish = vi.fn((_subject: string, _b: Uint8Array) => {});
vi.mock('../../state/nats', () => ({ getBus: () => ({ subscribe: busSubscribe, publish: busPublish }) }));

const mountFn = vi.fn();
const cleanupFn = vi.fn();

beforeEach(() => {
  mountFn.mockReset();
  cleanupFn.mockReset();
  busSubscribe.mockClear();
  busPublish.mockClear();
  mountFn.mockImplementation((el: HTMLElement) => {
    const p = el.ownerDocument.createElement('p');
    p.textContent = 'module-loaded';
    el.appendChild(p);
    return cleanupFn;
  });
  (globalThis as any).__waypointImport = vi.fn().mockResolvedValue({
    default: { mount: mountFn },
  });
});

describe('ModulePanel', () => {
  it('calls the bundle.mount with a container and ctx', async () => {
    render(<ModulePanel moduleId="power-monitor" roverId="rover-01" />);
    await waitFor(() => expect(mountFn).toHaveBeenCalled());
    const ctx = mountFn.mock.calls[0][1];
    expect(ctx).toMatchObject({
      roverId: 'rover-01',
    });
    expect(typeof ctx.subscribe).toBe('function');
    const off = ctx.subscribe('waypoint.r1.module.umr.stats', () => {});
    expect(busSubscribe).toHaveBeenCalledWith(
      'waypoint.r1.module.umr.stats',
      expect.any(Function),
    );
    expect(typeof off).toBe('function');
    expect(screen.getByText('module-loaded')).toBeInTheDocument();
  });

  it('exposes a publish scoped to the module subtree', async () => {
    render(<ModulePanel moduleId="so100" roverId="rover-01" />);
    await waitFor(() => expect(mountFn).toHaveBeenCalled());
    const ctx = mountFn.mock.calls[0][1];
    expect(typeof ctx.publish).toBe('function');
    ctx.publish('waypoint.rover-01.module.so100.command', new Uint8Array([1]));
    expect(busPublish).toHaveBeenCalledWith(
      'waypoint.rover-01.module.so100.command',
      expect.any(Uint8Array),
    );
  });

  it('refuses publishing outside the module subtree', async () => {
    render(<ModulePanel moduleId="so100" roverId="rover-01" />);
    await waitFor(() => expect(mountFn).toHaveBeenCalled());
    const ctx = mountFn.mock.calls[0][1];
    expect(() => ctx.publish('waypoint.rover-01.cmd.drive', new Uint8Array([1]))).toThrow();
    expect(busPublish).not.toHaveBeenCalled();
  });

  it('runs the returned cleanup on unmount', async () => {
    const { unmount } = render(<ModulePanel moduleId="power-monitor" roverId="rover-01" />);
    await waitFor(() => expect(mountFn).toHaveBeenCalled());
    unmount();
    expect(cleanupFn).toHaveBeenCalled();
  });

  it('renders a plain-text error if the bundle fails to load', async () => {
    (globalThis as any).__waypointImport = vi.fn().mockRejectedValue(new Error('boom'));
    render(<ModulePanel moduleId="broken" roverId="rover-01" />);
    await waitFor(() => expect(screen.getByText(/Failed to load module/)).toBeInTheDocument());
  });
});
