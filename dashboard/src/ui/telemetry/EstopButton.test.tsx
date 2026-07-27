import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { EstopButton } from './EstopButton';

describe('EstopButton', () => {
  it('engages when not estopped', () => {
    const onEngage = vi.fn();
    render(<EstopButton mode="manual" offline={false} onEngage={onEngage} onClear={() => {}} />);
    const btn = screen.getByRole('button', { name: /emergency stop/i });
    btn.click();
    expect(onEngage).toHaveBeenCalled();
  });

  it('shows Clear and calls onClear when estopped', () => {
    const onClear = vi.fn();
    render(<EstopButton mode="estop" offline={false} onEngage={() => {}} onClear={onClear} />);
    const btn = screen.getByRole('button', { name: /clear e-stop/i });
    btn.click();
    expect(onClear).toHaveBeenCalled();
  });

  it('shows "Clearing…" and disables while clearing', () => {
    render(<EstopButton mode="estop" offline={false} clearing onEngage={() => {}} onClear={() => {}} />);
    expect(screen.getByRole('button', { name: /clearing/i })).toBeDisabled();
  });

  it('disables engage when offline', () => {
    render(<EstopButton mode="manual" offline onEngage={() => {}} onClear={() => {}} />);
    expect(screen.getByRole('button', { name: /emergency stop/i })).toBeDisabled();
  });
});
