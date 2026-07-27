import { describe, expect, it } from 'vitest';
import { displayInitials } from './initials';

describe('displayInitials', () => {
  it('derives two initials from a dotted email local-part', () => {
    expect(displayInitials({ mode: 'proxy', email: 'jamie.rivera@example.com' })).toBe('JR');
  });

  it('falls back to the first two characters for single-word local-parts', () => {
    expect(displayInitials({ mode: 'proxy', email: 'alice@example.com' })).toBe('AL');
  });

  it('takes only the first two parts when many dots are present', () => {
    expect(displayInitials({ mode: 'proxy', email: 'a.b.c.d@example.com' })).toBe('AB');
  });

  it('derives from roverId in local mode when no email is set', () => {
    expect(displayInitials({ mode: 'local', roverId: 'rover-dev' })).toBe('RD');
  });

  it('falls back to the first two characters of a single-token roverId', () => {
    expect(displayInitials({ mode: 'local', roverId: 'rover' })).toBe('RO');
  });

  it('returns empty when Me has neither email nor roverId', () => {
    expect(displayInitials({ mode: 'offline' })).toBe('');
  });

  it('returns empty for null/undefined', () => {
    expect(displayInitials(null)).toBe('');
    expect(displayInitials(undefined)).toBe('');
  });
});
