import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TimelineScrubber } from './TimelineScrubber';

const S = 1_000_000_000n;

function renderScrubber(onSeek: (t: bigint) => void, cursorNs = 50n * S) {
  render(
    <TimelineScrubber
      startNs={0n}
      endNs={100n * S}
      cursorNs={cursorNs}
      selection={null}
      onSeek={onSeek}
      onSelect={() => {}}
    />,
  );
  return screen.getByRole('slider', { name: /timeline/i });
}

describe('TimelineScrubber', () => {
  it('exposes the cursor as a slider', () => {
    const slider = renderScrubber(() => {});
    expect(slider).toHaveAttribute('aria-valuenow', '50');
    expect(slider).toHaveAttribute('aria-valuemax', '100');
    expect(slider).toHaveAttribute('tabindex', '0');
  });

  it('seeks with arrow keys, paging, and Home/End', () => {
    const onSeek = vi.fn();
    const slider = renderScrubber(onSeek);
    fireEvent.keyDown(slider, { key: 'ArrowRight' });
    expect(onSeek).toHaveBeenLastCalledWith(51n * S);
    fireEvent.keyDown(slider, { key: 'ArrowLeft' });
    expect(onSeek).toHaveBeenLastCalledWith(49n * S);
    fireEvent.keyDown(slider, { key: 'PageUp' });
    expect(onSeek).toHaveBeenLastCalledWith(60n * S);
    fireEvent.keyDown(slider, { key: 'Home' });
    expect(onSeek).toHaveBeenLastCalledWith(0n);
    fireEvent.keyDown(slider, { key: 'End' });
    expect(onSeek).toHaveBeenLastCalledWith(100n * S);
  });

  it('clamps keyboard seeks to the episode range', () => {
    const onSeek = vi.fn();
    const slider = renderScrubber(onSeek, 0n);
    fireEvent.keyDown(slider, { key: 'ArrowLeft' });
    expect(onSeek).toHaveBeenLastCalledWith(0n);
  });
});
