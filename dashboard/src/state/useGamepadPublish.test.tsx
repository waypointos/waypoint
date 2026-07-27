import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

const publish = vi.hoisted(() => vi.fn());
vi.mock('./nats', () => ({ getBus: () => ({ publish }) }));

import { useGamepadPublish } from './useGamepadPublish';
import { GamepadSnapshot } from '../../../protocol/gen/ts/messages/gamepad_pb';

function fakePad(axes: number[], buttons: number[] = [], id = '') {
  return { id, axes, buttons: buttons.map((v) => ({ value: v, pressed: v > 0.5 })) };
}

function firstSnap() {
  return GamepadSnapshot.fromBinary(publish.mock.calls[0][1] as Uint8Array);
}

beforeEach(() => { vi.useFakeTimers(); publish.mockClear(); });
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('useGamepadPublish', () => {
  it('stays silent when no gamepad is connected', () => {
    vi.stubGlobal('navigator', { getGamepads: () => [null, null, null, null] });
    renderHook(() => useGamepadPublish('r1'));
    act(() => { vi.advanceTimersByTime(200); });
    expect(publish).not.toHaveBeenCalled();
  });

  it('publishes ~50Hz to cmd.gamepad while a pad is active', () => {
    vi.stubGlobal('navigator', { getGamepads: () => [fakePad([0.5, 0, 0, 0], [0, 1])] });
    renderHook(() => useGamepadPublish('r1'));
    act(() => { vi.advanceTimersByTime(200); });
    expect(publish.mock.calls.length).toBeGreaterThanOrEqual(9);
    expect(publish.mock.calls[0][0]).toBe('waypoint.r1.cmd.gamepad');
  });

  // WebKit raw front-button order is [.,.,.,., L2, R2, L1, R1]; we must republish
  // it as the W3C [L1, R1, L2, R2] so triggers (6/7) and bumpers (4/5) are right.
  it('normalizes WebKit Extended Gamepad trigger/bumper order', () => {
    //                         idx: 0 1 2 3  4=L2  5=R2  6=L1  7=R1
    const pad = fakePad([0, 0, 0, 0], [0, 0, 0, 0, 0.9, 0.0, 1.0, 0.0],
      'DualSense Wireless Controller Extended Gamepad');
    vi.stubGlobal('navigator', { getGamepads: () => [pad] });
    renderHook(() => useGamepadPublish('r1'));
    act(() => { vi.advanceTimersByTime(20); });
    const snap = firstSnap();
    // triggers come from canonical 6/7 -> the analog L2/R2 values after the swap
    expect(snap.triggers[0]).toBeCloseTo(0.9, 5);
    expect(snap.triggers[1]).toBeCloseTo(0.0, 5);
    // bumpers L1/R1 now at canonical 4/5
    expect(snap.buttons[4]).toBe(true);  // L1 pressed
    expect(snap.buttons[5]).toBe(false); // R1
    expect(snap.buttons[6]).toBe(true);  // L2 (0.9 > 0.5)
  });

  it('leaves a standard (Chromium) pad button order untouched', () => {
    //                              idx: 4=L1   5=R1  6=L2  7=R2  (already standard)
    const pad = fakePad([0, 0, 0, 0], [0, 0, 0, 0, 1.0, 0.0, 0.3, 0.0],
      'DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)');
    vi.stubGlobal('navigator', { getGamepads: () => [pad] });
    renderHook(() => useGamepadPublish('r1'));
    act(() => { vi.advanceTimersByTime(20); });
    const snap = firstSnap();
    expect(snap.buttons[4]).toBe(true);          // L1 untouched
    expect(snap.triggers[0]).toBeCloseTo(0.3, 5); // L2 analog from 6, untouched
  });
});
