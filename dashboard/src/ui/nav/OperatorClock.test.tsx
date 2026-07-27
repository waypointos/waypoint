import { describe, expect, it } from 'vitest';
import { format } from './OperatorClock';

describe('OperatorClock.format', () => {
  it('renders HH:MM:SS LOCAL', () => {
    const d = new Date('2026-05-11T15:42:18');
    expect(format(d)).toMatch(/^15:42:18 LOCAL$/);
  });

  it('zero-pads', () => {
    const d = new Date('2026-05-11T03:04:05');
    expect(format(d)).toBe('03:04:05 LOCAL');
  });
});
