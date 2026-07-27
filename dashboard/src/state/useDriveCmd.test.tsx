import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

const publish = vi.hoisted(() => vi.fn());
vi.mock('./nats', () => ({ getBus: () => ({ publish }) }));

import { useDriveCmd } from './useDriveCmd';

beforeEach(() => { vi.useFakeTimers(); publish.mockClear(); });
afterEach(() => { vi.useRealTimers(); });

type Stick = { vx: number; yaw: number };

describe('useDriveCmd', () => {
  it('stays silent when the stick is centred', () => {
    renderHook(() => useDriveCmd('r1', { vx: 0, yaw: 0 }));
    act(() => { vi.advanceTimersByTime(1000); });
    expect(publish).not.toHaveBeenCalled();
  });

  it('publishes ~50 Hz while the stick is non-zero', () => {
    renderHook(() => useDriveCmd('r1', { vx: 0.5, yaw: 0 }));
    act(() => { vi.advanceTimersByTime(200); });
    expect(publish).toHaveBeenCalledTimes(10);
    expect(publish.mock.calls[0][0]).toBe('waypoint.r1.cmd.drive');
  });

  it('keeps emitting through the grace window after release, then stops', () => {
    const { rerender } = renderHook(
      ({ s }: { s: Stick }) => useDriveCmd('r1', s),
      { initialProps: { s: { vx: 0.5, yaw: 0 } } },
    );
    act(() => { vi.advanceTimersByTime(40); }); // prime: 2 frames published
    publish.mockClear();

    rerender({ s: { vx: 0, yaw: 0 } });
    act(() => { vi.advanceTimersByTime(200); }); // within grace
    const inGrace = publish.mock.calls.length;
    expect(inGrace).toBeGreaterThan(0);

    publish.mockClear();
    act(() => { vi.advanceTimersByTime(500); }); // past grace
    expect(publish).not.toHaveBeenCalled();
  });

  it('resumes immediately when the stick leaves centre again', () => {
    const { rerender } = renderHook(
      ({ s }: { s: Stick }) => useDriveCmd('r1', s),
      { initialProps: { s: { vx: 0, yaw: 0 } } },
    );
    act(() => { vi.advanceTimersByTime(1000); });
    expect(publish).not.toHaveBeenCalled();

    rerender({ s: { vx: 0, yaw: 0.3 } });
    act(() => { vi.advanceTimersByTime(40); });
    expect(publish.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it('never publishes when paused', () => {
    renderHook(() => useDriveCmd('r1', { vx: 1, yaw: 1 }, { paused: true }));
    act(() => { vi.advanceTimersByTime(1000); });
    expect(publish).not.toHaveBeenCalled();
  });
});
