import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { RecordControl } from './RecordControl';

const startRecord = vi.fn();
const stopRecord = vi.fn();
vi.mock('../../state/roverCommands', () => ({
  startRecord: (...a: unknown[]) => startRecord(...a),
  stopRecord: (...a: unknown[]) => stopRecord(...a),
}));

describe('RecordControl', () => {
  it('starts an episode with a task label', () => {
    render(<RecordControl roverId="r1" recorder={{ state: 'idle', episodeId: '', elapsedS: 0, bytes: 0, canStart: true, reason: '' }} />);
    fireEvent.click(screen.getByRole('button', { name: /rec/i }));
    fireEvent.change(screen.getByLabelText(/task label/i), { target: { value: 'push the block' } });
    fireEvent.click(screen.getByRole('button', { name: /start/i }));
    expect(startRecord).toHaveBeenCalledWith('r1', 'push the block');
  });

  it('disables with a reason hint when the recorder cannot start', () => {
    render(<RecordControl roverId="r1" recorder={{ state: 'idle', episodeId: '', elapsedS: 0, bytes: 0, canStart: false, reason: 'low disk: 120 MiB free' }} />);
    const btn = screen.getByRole('button', { name: /rec/i });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute('title', expect.stringContaining('low disk'));
  });

  it('shows elapsed time while recording and prompts for outcome on stop', () => {
    render(<RecordControl roverId="r1" recorder={{ state: 'recording', episodeId: 'ep-1', elapsedS: 75, bytes: 1048576, canStart: false, reason: '' }} />);
    expect(screen.getByText(/1:15/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /stop/i }));
    fireEvent.click(screen.getByRole('button', { name: /success/i }));
    expect(stopRecord).toHaveBeenCalledWith('r1', true, '');
  });

  it('records a failure outcome', () => {
    render(<RecordControl roverId="r1" recorder={{ state: 'recording', episodeId: 'ep-1', elapsedS: 5, bytes: 0, canStart: false, reason: '' }} />);
    fireEvent.click(screen.getByRole('button', { name: /stop/i }));
    fireEvent.click(screen.getByRole('button', { name: /failure/i }));
    expect(stopRecord).toHaveBeenCalledWith('r1', false, '');
  });
});
