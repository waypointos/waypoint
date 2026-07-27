import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SpeedPresets } from './SpeedPresets';

describe('SpeedPresets', () => {
  it('calls onPreset with the clicked preset', () => {
    const onPreset = vi.fn();
    render(<SpeedPresets preset="cruise" cap={1} onPreset={onPreset} onCap={() => {}} />);
    screen.getByRole('button', { name: 'Turbo' }).click();
    expect(onPreset).toHaveBeenCalledWith('turbo');
  });

  it('marks the active preset', () => {
    render(<SpeedPresets preset="slow" cap={1} onPreset={() => {}} onCap={() => {}} />);
    expect(screen.getByRole('button', { name: 'Slow' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('emits cap as a 0..1 fraction', () => {
    const onCap = vi.fn();
    render(<SpeedPresets preset="cruise" cap={1} onPreset={() => {}} onCap={onCap} />);
    fireEvent.change(screen.getByRole('slider'), { target: { value: '50' } });
    expect(onCap).toHaveBeenCalledWith(0.5);
  });
});
