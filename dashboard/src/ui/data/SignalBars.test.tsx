import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SignalBars } from './SignalBars';

describe('SignalBars', () => {
  it('renders N/A when value is null', () => {
    render(<SignalBars value={null} />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
  });

  it('renders 3 active + 1 inactive bar for value=3 max=4', () => {
    const { container } = render(<SignalBars value={3} />);
    const active = container.querySelectorAll('[data-active="true"]');
    const inactive = container.querySelectorAll('[data-active="false"]');
    expect(active.length).toBe(3);
    expect(inactive.length).toBe(1);
  });

  it('clamps value above max to max', () => {
    const { container } = render(<SignalBars value={10} />);
    const active = container.querySelectorAll('[data-active="true"]');
    expect(active.length).toBe(4);
  });

  it('renders 0/4 (all inactive) for value=0', () => {
    const { container } = render(<SignalBars value={0} />);
    const active = container.querySelectorAll('[data-active="true"]');
    const inactive = container.querySelectorAll('[data-active="false"]');
    expect(active.length).toBe(0);
    expect(inactive.length).toBe(4);
  });

  it('shows "n/4" label when showLabel', () => {
    render(<SignalBars value={2} showLabel />);
    expect(screen.getByText('2/4')).toBeInTheDocument();
  });
});
