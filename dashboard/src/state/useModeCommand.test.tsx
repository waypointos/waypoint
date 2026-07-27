import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import type { Mode as DisplayMode } from '@/ui/telemetry/ModeIndicator';

const mocks = vi.hoisted(() => ({
  setMode: vi.fn(),
  estop: vi.fn(),
  recover: vi.fn(),
}));
vi.mock('./roverCommands', () => mocks);

import { useModeCommand } from './useModeCommand';

beforeEach(() => { vi.useFakeTimers(); mocks.setMode.mockClear(); mocks.estop.mockClear(); mocks.recover.mockClear(); });
afterEach(() => { vi.useRealTimers(); });

describe('useModeCommand', () => {
  it('requestMode sets pending and calls setMode', () => {
    const { result } = renderHook(({ m }) => useModeCommand('r1', m), { initialProps: { m: 'safe' as DisplayMode } });
    act(() => result.current.requestMode('manual'));
    expect(mocks.setMode).toHaveBeenCalledWith('r1', 'manual');
    expect(result.current.pending).toBe('manual');
  });

  it('clears pending when currentMode reaches the requested mode', () => {
    const { result, rerender } = renderHook(({ m }) => useModeCommand('r1', m), { initialProps: { m: 'safe' as DisplayMode } });
    act(() => result.current.requestMode('manual'));
    rerender({ m: 'manual' });
    expect(result.current.pending).toBeNull();
  });

  it('clears pending after the timeout when no event.mode arrives', () => {
    const { result } = renderHook(({ m }) => useModeCommand('r1', m), { initialProps: { m: 'safe' as DisplayMode } });
    act(() => result.current.requestMode('manual'));
    act(() => { vi.advanceTimersByTime(1500); });
    expect(result.current.pending).toBeNull();
  });

  it('requestRecover is pending "clearing" and clears once out of estop', () => {
    const { result, rerender } = renderHook(({ m }) => useModeCommand('r1', m), { initialProps: { m: 'estop' as DisplayMode } });
    act(() => result.current.requestRecover());
    expect(mocks.recover).toHaveBeenCalledWith('r1');
    expect(result.current.pending).toBe('clearing');
    rerender({ m: 'safe' });
    expect(result.current.pending).toBeNull();
  });
});
