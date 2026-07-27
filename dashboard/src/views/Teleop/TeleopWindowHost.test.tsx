import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';

vi.mock('@/state/nats', () => ({ getBus: () => ({ subscribe: () => () => {}, publish: vi.fn() }) }));
vi.mock('@/state/mode', () => ({ useMe: () => ({ mode: 'local' }) }));

const mounted = vi.hoisted(() => vi.fn());
beforeEach(() => {
  mounted.mockClear();
  (globalThis as any).__waypointImport = vi.fn(async () => ({
    default: { mount: (el: HTMLElement, ctx: any) => { mounted(ctx.roverId); el.textContent = 'arm'; return () => {}; } },
  }));
});

import { TeleopWindowHost } from './TeleopWindowHost';

const win = { moduleId: 'so100', windowId: 'w-so100', label: 'Arm', entry: 'teleop.js', bindings: [] };

describe('TeleopWindowHost', () => {
  it('renders one toggle chip per advertised window and mounts on open', async () => {
    render(<TeleopWindowHost roverId="r1" windows={[win]} />);
    const chip = screen.getByRole('button', { name: /arm/i });
    expect(chip).toHaveAttribute('aria-pressed', 'false');
    await act(async () => { fireEvent.click(chip); });
    expect(mounted).toHaveBeenCalledWith('r1');
    expect(chip).toHaveAttribute('aria-pressed', 'true');
  });

  it('rejects out-of-subtree publishes from the window context', async () => {
    let caught: string | null = null;
    (globalThis as any).__waypointImport = vi.fn(async () => ({
      default: { mount: (_el: HTMLElement, ctx: any) => {
        try { ctx.publish('waypoint.r1.cmd.drive', new Uint8Array()); } catch (e: any) { caught = e.message; }
        return () => {};
      } },
    }));
    render(<TeleopWindowHost roverId="r1" windows={[win]} />);
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /arm/i })); });
    expect(caught).toMatch(/may not publish/);
  });
});
