import { describe, expect, it } from 'vitest';
import { newTransport, tick, play, pause, setSpeed, seek, keyframeBefore, SPEEDS } from './transport';

describe('transport', () => {
  it('advances by wall time times speed while playing', () => {
    let t = play(newTransport(0n));
    t = setSpeed(t, 2);
    t = tick(t, 100, 10_000_000_000n); // 100ms wall at 2x = 200ms
    expect(t.cursorNs).toBe(200_000_000n);
  });

  it('does not advance while paused', () => {
    let t = newTransport(0n);
    t = tick(t, 100, 10_000_000_000n);
    expect(t.cursorNs).toBe(0n);
    expect(t.playing).toBe(false);
  });

  it('clamps at the end and pauses', () => {
    let t = play(newTransport(0n));
    t = tick(t, 999_999, 500_000_000n);
    expect(t.cursorNs).toBe(500_000_000n);
    expect(t.playing).toBe(false);
  });

  it('seek clamps into the episode range', () => {
    const t = newTransport(1_000n);
    expect(seek(t, 5n, 1_000n, 2_000n).cursorNs).toBe(1_000n);
    expect(seek(t, 9_999n, 1_000n, 2_000n).cursorNs).toBe(2_000n);
    expect(seek(t, 1_500n, 1_000n, 2_000n).cursorNs).toBe(1_500n);
  });

  it('offers the locked speed steps and pause/resume round-trips', () => {
    expect(SPEEDS).toEqual([0.25, 0.5, 1, 2, 4]);
    const t = pause(play(newTransport(0n)));
    expect(t.playing).toBe(false);
  });

  it('keyframeBefore finds the greatest key time at or before t', () => {
    const keys = [10n, 50n, 90n];
    expect(keyframeBefore(keys, 60n)).toBe(50n);
    expect(keyframeBefore(keys, 50n)).toBe(50n);
    expect(keyframeBefore(keys, 5n)).toBeNull();
  });
});
