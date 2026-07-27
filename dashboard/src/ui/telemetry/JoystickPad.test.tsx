// dashboard/src/ui/telemetry/JoystickPad.test.tsx
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
import { JoystickPad } from './JoystickPad';

describe('JoystickPad', () => {
  it('emits zero on mouseup (recenter)', () => {
    const onChange = vi.fn();
    const { container } = render(<JoystickPad onChange={onChange} />);
    const pad = container.querySelector('[data-joy]') as HTMLElement;
    fireEvent.pointerDown(pad, { clientX: 100, clientY: 100, pointerId: 1 });
    fireEvent.pointerUp(pad, { clientX: 100, clientY: 100, pointerId: 1 });
    const last = onChange.mock.calls.at(-1)?.[0];
    expect(last).toEqual({ vx: 0, yaw: 0 });
  });
});
