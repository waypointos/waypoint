// dashboard/src/state/useTelemetry.test.tsx
import { describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';

// Mock getBus before importing useTelemetry.
const subscribers = new Map<string, (b: Uint8Array) => void>();
vi.mock('./nats', () => ({
  getBus: () => ({
    subscribe: (subject: string, h: (b: Uint8Array) => void) => {
      subscribers.set(subject, h);
      return () => subscribers.delete(subject);
    },
    publish: () => {},
  }),
}));

const { useTelemetry } = await import('./useTelemetry');

function Probe() {
  const v = useTelemetry('test', (b) => new TextDecoder().decode(b));
  return <span data-testid="v">{v ?? '—'}</span>;
}

describe('useTelemetry', () => {
  it('renders the latest decoded message', async () => {
    const { getByTestId } = render(<Probe />);
    subscribers.get('test')!(new TextEncoder().encode('hello'));
    await waitFor(() => expect(getByTestId('v').textContent).toBe('hello'));
  });
});
