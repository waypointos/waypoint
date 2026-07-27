// dashboard/src/state/useAlerts.test.tsx
//
// Happy-path coverage: raised → list contains it → resolved → list empty.
// We stub the bus subscribe/publish path the same way useTelemetry.test.tsx
// does, and stub /api/alerts seed so the hook can mount cleanly.
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, render, waitFor } from '@testing-library/react';
import { AlertRaised, AlertResolved } from '../../../protocol/gen/ts/messages/alerts_pb';
import { Severity } from '../../../protocol/gen/ts/messages/common_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { pbToBinary } from './protobuf';

// In-memory subscriber registry shared across the test.
const subscribers = new Map<string, Set<(b: Uint8Array) => void>>();

vi.mock('./nats', () => ({
  getBus: () => ({
    subscribe: (subject: string, h: (b: Uint8Array) => void) => {
      let set = subscribers.get(subject);
      if (!set) {
        set = new Set();
        subscribers.set(subject, set);
      }
      set.add(h);
      return () => {
        set?.delete(h);
      };
    },
    publish: () => {},
  }),
}));

vi.mock('./useFleet', () => ({
  useFleet: () => ({
    rovers: [{ id: 'rover-a', name: 'A', role: 'control' }],
    loading: false,
  }),
  isOnline: () => true,
}));

// fetch is called once on mount to seed; return an empty list.
const fetchMock = vi.fn(async () => ({
  ok: true,
  status: 200,
  json: async () => [],
})) as unknown as typeof fetch;

beforeEach(() => {
  subscribers.clear();
  vi.stubGlobal('fetch', fetchMock);
});

const { useAlerts } = await import('./useAlerts');

function Probe() {
  const { alerts, counts } = useAlerts();
  return (
    <div>
      <span data-testid="count">{alerts.length}</span>
      <span data-testid="critical">{counts.critical}</span>
      <span data-testid="first">{alerts[0]?.message ?? ''}</span>
    </div>
  );
}

function dispatch(subject: string, bytes: Uint8Array) {
  const hs = subscribers.get(subject);
  if (!hs) return;
  for (const h of hs) h(bytes);
}

describe('useAlerts', () => {
  it('inserts on raised then drops on resolved', async () => {
    const { getByTestId } = render(<Probe />);

    // Wait for the seed effect to settle and the NATS effect to attach.
    await waitFor(() => {
      expect(subscribers.has('waypoint.rover-a.alert.raised')).toBe(true);
    });

    const raised = new AlertRaised({
      id: 'a-1',
      source: 'core',
      severity: Severity.FAULT,
      code: 'heartbeat_lost',
      message: 'C++ core stopped pinging',
      t: Timestamp.fromDate(new Date(Date.now() - 5_000)),
    });
    act(() => {
      dispatch('waypoint.rover-a.alert.raised', pbToBinary(raised));
    });

    await waitFor(() => {
      expect(getByTestId('count').textContent).toBe('1');
      expect(getByTestId('critical').textContent).toBe('1');
      expect(getByTestId('first').textContent).toBe('C++ core stopped pinging');
    });

    const resolved = new AlertResolved({
      id: 'a-1',
      reason: 'auto',
      autoResolved: true,
      t: Timestamp.fromDate(new Date()),
    });
    act(() => {
      dispatch('waypoint.rover-a.alert.resolved', pbToBinary(resolved));
    });

    await waitFor(() => {
      expect(getByTestId('count').textContent).toBe('0');
      expect(getByTestId('critical').textContent).toBe('0');
    });
  });
});
