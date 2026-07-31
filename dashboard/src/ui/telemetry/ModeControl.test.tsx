import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ModeControl } from './ModeControl';

describe('ModeControl', () => {
  it('calls onSelect with the clicked target', () => {
    const onSelect = vi.fn();
    render(<ModeControl mode="safe" pending={null} onSelect={onSelect} />);
    screen.getByRole('button', { name: 'Manual' }).click();
    expect(onSelect).toHaveBeenCalledWith('manual');
  });

  it('offers Auto and calls onSelect with autonomous', () => {
    const onSelect = vi.fn();
    render(<ModeControl mode="manual" pending={null} onSelect={onSelect} />);
    screen.getByRole('button', { name: 'Auto' }).click();
    expect(onSelect).toHaveBeenCalledWith('autonomous');
  });

  it('marks Auto active while the rover is autonomous', () => {
    render(<ModeControl mode="autonomous" pending={null} onSelect={() => {}} />);
    expect(screen.getByRole('button', { name: 'Auto' }).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', { name: 'Manual' }).getAttribute('aria-pressed')).toBe('false');
  });

  it('marks the current mode active', () => {
    render(<ModeControl mode="manual" pending={null} onSelect={() => {}} />);
    expect(screen.getByRole('button', { name: 'Manual' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('disables selection while estopped (must clear first)', () => {
    render(<ModeControl mode="estop" pending={null} onSelect={() => {}} />);
    expect(screen.getByRole('button', { name: 'Manual' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Auto' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Safe' })).toBeDisabled();
  });

  it('disables selection when disabled prop is set (monitor/offline)', () => {
    render(<ModeControl mode="safe" pending={null} disabled onSelect={() => {}} />);
    expect(screen.getByRole('button', { name: 'Manual' })).toBeDisabled();
  });

  it('flags the pending target', () => {
    render(<ModeControl mode="safe" pending="manual" onSelect={() => {}} />);
    expect(screen.getByRole('button', { name: 'Manual' }).getAttribute('data-pending')).toBe('true');
  });
});
