import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { FloatingWindow } from './FloatingWindow';

describe('FloatingWindow', () => {
  it('renders the title and children', () => {
    render(<FloatingWindow title="SO-100 ARM"><p>body</p></FloatingWindow>);
    expect(screen.getByText('SO-100 ARM')).toBeInTheDocument();
    expect(screen.getByText('body')).toBeInTheDocument();
  });

  it('calls onClose when the close control is clicked', () => {
    const onClose = vi.fn();
    render(<FloatingWindow title="W" onClose={onClose}><p>body</p></FloatingWindow>);
    fireEvent.click(screen.getByRole('button', { name: /close/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('collapses and expands the body', () => {
    render(<FloatingWindow title="W"><p>body</p></FloatingWindow>);
    fireEvent.click(screen.getByRole('button', { name: /collapse/i }));
    expect(screen.queryByText('body')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /expand/i }));
    expect(screen.getByText('body')).toBeInTheDocument();
  });

  it('moves when the title bar is dragged', () => {
    render(
      <FloatingWindow title="W" initialPosition={{ x: 100, y: 100 }} initialSize={{ width: 200, height: 150 }}>
        <p>body</p>
      </FloatingWindow>,
    );
    const win = screen.getByTestId('floating-window');
    const bar = screen.getByTestId('floating-window-titlebar');
    fireEvent.pointerDown(bar, { clientX: 150, clientY: 110 });
    fireEvent.pointerMove(window, { clientX: 180, clientY: 140 });
    fireEvent.pointerUp(window, { clientX: 180, clientY: 140 });
    expect(win.style.left).toBe('130px'); // 100 + (180-150)
    expect(win.style.top).toBe('130px');  // 100 + (140-110)
  });
});
