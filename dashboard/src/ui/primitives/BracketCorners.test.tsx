// dashboard/src/ui/primitives/BracketCorners.test.tsx
import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { BracketCorners } from './BracketCorners';

describe('BracketCorners', () => {
  it('renders two corner spans by default (tl + br)', () => {
    const { container } = render(
      <div style={{ position: 'relative' }}>
        <BracketCorners />
      </div>,
    );
    const corners = container.querySelectorAll('[data-bracket-corner]');
    expect(corners.length).toBe(2);
  });

  it('renders four corners when full', () => {
    const { container } = render(
      <div style={{ position: 'relative' }}>
        <BracketCorners full />
      </div>,
    );
    expect(container.querySelectorAll('[data-bracket-corner]').length).toBe(4);
  });
});
