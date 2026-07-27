import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

vi.mock('@/lib/gamepad', () => ({ useGamepadStick: () => ({ vx: 0.5, yaw: -0.5 }) }));

import { useDriveInput } from './useDriveInput';

beforeEach(() => {
  (navigator as any).getGamepads = () => [];
});

describe('useDriveInput', () => {
  it('defaults to the virtual source and reflects setVirtual', () => {
    const { result } = renderHook(() => useDriveInput());
    expect(result.current.source).toBe('virtual');
    act(() => result.current.setVirtual({ vx: 0.2, yaw: 0.1 }));
    expect(result.current.stick).toEqual({ vx: 0.2, yaw: 0.1 });
  });

  it('switches to gamepad on gamepadconnected and uses its vector', () => {
    const { result } = renderHook(() => useDriveInput());
    act(() => { window.dispatchEvent(new Event('gamepadconnected')); });
    expect(result.current.source).toBe('gamepad');
    expect(result.current.gamepadConnected).toBe(true);
    expect(result.current.stick).toEqual({ vx: 0.5, yaw: -0.5 });
  });

  it('reverts to virtual when the last gamepad disconnects', () => {
    const { result } = renderHook(() => useDriveInput());
    act(() => { window.dispatchEvent(new Event('gamepadconnected')); });
    act(() => { window.dispatchEvent(new Event('gamepaddisconnected')); });
    expect(result.current.source).toBe('virtual');
  });
});
