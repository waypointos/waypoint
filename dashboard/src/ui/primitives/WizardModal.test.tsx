import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { WizardModal, type WizardStep } from './WizardModal';

const STEPS: WizardStep[] = [
  { number: '01', label: 'IDENTIFY', state: 'done' },
  { number: '02', label: 'GENERATE', state: 'current' },
  { number: '03', label: 'FLASH', state: 'upcoming' },
];

describe('WizardModal', () => {
  it('renders nothing when closed', () => {
    render(
      <WizardModal open={false} onClose={() => {}} steps={STEPS}>
        <p>body content</p>
      </WizardModal>,
    );
    expect(screen.queryByText('body content')).toBeNull();
    expect(screen.queryByTestId('wizard-modal-scrim')).toBeNull();
  });

  it('renders title, all steps, body content, and footer when open', () => {
    render(
      <WizardModal
        open
        onClose={() => {}}
        title="ENROLL ROVER"
        steps={STEPS}
        footer={<button>Next</button>}
      >
        <p>body content</p>
      </WizardModal>,
    );
    expect(screen.getByText('ENROLL ROVER')).toBeInTheDocument();
    expect(screen.getByText('body content')).toBeInTheDocument();
    expect(screen.getByText('Next')).toBeInTheDocument();
    expect(screen.getByText('IDENTIFY')).toBeInTheDocument();
    expect(screen.getByText('GENERATE')).toBeInTheDocument();
    expect(screen.getByText('FLASH')).toBeInTheDocument();
  });

  it('highlights the current step item with a "current" class', () => {
    render(
      <WizardModal open onClose={() => {}} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    const currentItem = screen.getByRole('listitem', { current: 'step' });
    expect(currentItem.className).toMatch(/current/);
    expect(currentItem.dataset.stepNumber).toBe('02');
  });

  it('calls onClose when Escape is pressed', () => {
    const onClose = vi.fn();
    render(
      <WizardModal open onClose={onClose} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('calls onClose when the scrim is clicked', () => {
    const onClose = vi.fn();
    render(
      <WizardModal open onClose={onClose} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    fireEvent.mouseDown(screen.getByTestId('wizard-modal-scrim'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not call onClose when the modal panel is clicked', () => {
    const onClose = vi.fn();
    render(
      <WizardModal open onClose={onClose} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    fireEvent.mouseDown(screen.getByTestId('wizard-modal-panel'));
    expect(onClose).not.toHaveBeenCalled();
  });

  it('locks body scroll while open and restores on close', () => {
    const { rerender } = render(
      <WizardModal open onClose={() => {}} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    expect(document.body.style.overflow).toBe('hidden');
    rerender(
      <WizardModal open={false} onClose={() => {}} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    expect(document.body.style.overflow).toBe('');
  });

  it('re-locks body scroll on reopen after close', () => {
    const { rerender } = render(
      <WizardModal open onClose={() => {}} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    rerender(
      <WizardModal open={false} onClose={() => {}} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    expect(document.body.style.overflow).toBe('');
    rerender(
      <WizardModal open onClose={() => {}} steps={STEPS}>
        <p>body</p>
      </WizardModal>,
    );
    expect(document.body.style.overflow).toBe('hidden');
  });
});
