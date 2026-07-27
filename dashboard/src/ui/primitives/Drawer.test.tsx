import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Drawer } from './Drawer';

describe('Drawer', () => {
  it('renders nothing when closed', () => {
    render(
      <Drawer open={false} onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    expect(screen.queryByText('content')).toBeNull();
  });

  it('renders children, rail, and footer when open', () => {
    render(
      <Drawer
        open
        onClose={() => {}}
        rail={<div>rail-stuff</div>}
        footer={<div>foot-stuff</div>}
      >
        <p>content</p>
      </Drawer>,
    );
    expect(screen.getByText('content')).toBeInTheDocument();
    expect(screen.getByText('rail-stuff')).toBeInTheDocument();
    expect(screen.getByText('foot-stuff')).toBeInTheDocument();
  });

  it('calls onClose when Escape is pressed', () => {
    const onClose = vi.fn();
    render(
      <Drawer open onClose={onClose}>
        <p>content</p>
      </Drawer>,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('calls onClose when the scrim is clicked', () => {
    const onClose = vi.fn();
    render(
      <Drawer open onClose={onClose}>
        <p>content</p>
      </Drawer>,
    );
    fireEvent.mouseDown(screen.getByTestId('drawer-scrim'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not call onClose when the panel itself is clicked', () => {
    const onClose = vi.fn();
    render(
      <Drawer open onClose={onClose}>
        <p>content</p>
      </Drawer>,
    );
    fireEvent.mouseDown(screen.getByTestId('drawer-panel'));
    expect(onClose).not.toHaveBeenCalled();
  });

  it('locks body scroll while open and restores on close', () => {
    const { rerender } = render(
      <Drawer open onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    expect(document.body.style.overflow).toBe('hidden');
    rerender(
      <Drawer open={false} onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    expect(document.body.style.overflow).toBe('');
  });

  it('re-locks body scroll when reopened after close', () => {
    const { rerender } = render(
      <Drawer open onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    rerender(
      <Drawer open={false} onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    expect(document.body.style.overflow).toBe('');
    rerender(
      <Drawer open onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('restores body scroll when unmounted while open', () => {
    const { unmount } = render(
      <Drawer open onClose={() => {}}>
        <p>content</p>
      </Drawer>,
    );
    expect(document.body.style.overflow).toBe('hidden');
    unmount();
    expect(document.body.style.overflow).toBe('');
  });
});
