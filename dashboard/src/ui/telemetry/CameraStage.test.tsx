import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('./CameraView', () => ({
  CameraView: ({ label, hudShown }: { label: string; hudShown?: boolean }) => (
    <div data-testid={`cam-${label}`} data-hud={hudShown === false ? 'off' : 'on'}>
      {label}
    </div>
  ),
}));

import { CameraStage } from './CameraStage';

const streams = [
  { name: 'chassis-front', whepUrl: '/camera/chassis-front/whep' },
  { name: 'gripper', whepUrl: '/camera/gripper/whep' },
];

describe('CameraStage', () => {
  it('renders the first stream as main and the rest as PiPs', () => {
    render(<CameraStage streams={streams} />);
    expect(screen.getByTestId('cam-chassis-front').getAttribute('data-hud')).toBe('on');
    expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('off');
  });

  it('promotes a PiP to main on click', () => {
    render(<CameraStage streams={streams} />);
    fireEvent.click(screen.getByRole('button', { name: /switch to gripper/i }));
    expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('on');
    expect(screen.getByTestId('cam-chassis-front').getAttribute('data-hud')).toBe('off');
  });

  it('promotes a PiP via keyboard (Enter)', () => {
    render(<CameraStage streams={streams} />);
    const pip = screen.getByRole('button', { name: /switch to gripper/i });
    fireEvent.keyDown(pip, { key: 'Enter' });
    expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('on');
  });

  it('promotes a PiP via keyboard (Space)', () => {
    render(<CameraStage streams={streams} />);
    const pip = screen.getByRole('button', { name: /switch to gripper/i });
    fireEvent.keyDown(pip, { key: ' ' });
    expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('on');
  });

  it('renders nothing for an empty stream list', () => {
    const { container } = render(<CameraStage streams={[]} />);
    expect(container.firstChild).toBeNull();
  });

  describe('floatingPip', () => {
    it('swaps on a clean click (pointer down + up without movement)', () => {
      const { container } = render(<CameraStage streams={streams} floatingPip />);
      const pip = container.querySelector('[data-floating-pip]')!;
      fireEvent.pointerDown(pip, { clientX: 100, clientY: 100 });
      fireEvent.pointerUp(pip, { clientX: 100, clientY: 100 });
      expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('on');
    });

    it('does not swap when the pointer dragged past the threshold', () => {
      const { container } = render(<CameraStage streams={streams} floatingPip />);
      const pip = container.querySelector('[data-floating-pip]')!;
      fireEvent.pointerDown(pip, { clientX: 100, clientY: 100 });
      fireEvent.pointerMove(pip, { clientX: 130, clientY: 120 });
      fireEvent.pointerUp(pip, { clientX: 130, clientY: 120 });
      expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('off');
    });

    it('swaps via keyboard', () => {
      render(<CameraStage streams={streams} floatingPip />);
      fireEvent.keyDown(screen.getByRole('button', { name: /switch to gripper/i }), { key: 'Enter' });
      expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('on');
    });
  });

  it('falls back to the first stream if the active one disappears', () => {
    const { rerender } = render(<CameraStage streams={streams} />);
    fireEvent.click(screen.getByRole('button', { name: /switch to gripper/i }));
    expect(screen.getByTestId('cam-gripper').getAttribute('data-hud')).toBe('on');

    rerender(<CameraStage streams={[streams[0]]} />);
    expect(screen.getByTestId('cam-chassis-front').getAttribute('data-hud')).toBe('on');
    expect(screen.queryByTestId('cam-gripper')).toBeNull();
  });
});
