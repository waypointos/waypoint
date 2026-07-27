import { vi } from 'vitest';

vi.mock('@/state/mode', () => ({ useMe: () => ({ mode: 'local', isAdmin: false }) }));
vi.mock('@/state/useAlerts', () => ({
  acknowledgeAlert: () => {},
  resolveAlert: () => {},
}));
// Mock the bus so usePlatform never opens a real connection; no infra.platform
// message arrives, so the panel renders the baked-rover fallback (has drive).
vi.mock('@/state/nats', () => ({
  getBus: () => ({ subscribe: () => () => {}, publish: () => {} }),
}));

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { OverviewPanel, signalMetric } from './OverviewPanel';
import { RoverContextProvider, type RoverContextValue } from '../RoverContext';

function withCtx(overrides: Partial<RoverContextValue> = {}): RoverContextValue {
  return {
    id: 'demo',
    role: 'monitor',
    canControl: false,
    drive: null, power: null, sys: null, uplink: null, modules: [], image: null, motors: {},
    cameraNames: [], whepBase: '',
    roverAlerts: [],
    alertCounts: { info: 0, warning: 0, critical: 0 } as any,
    connection: { kind: 'offline' },
    mode: 'unknown',
    recorder: null,
    ...overrides,
  };
}

describe('OverviewPanel', () => {
  it('renders N/A states when telemetry is null', () => {
    render(
      <RoverContextProvider value={withCtx()}>
        <OverviewPanel />
      </RoverContextProvider>
    );
    expect(screen.getAllByText('N/A').length).toBeGreaterThan(0);
  });

  it('does not render the joystick', () => {
    render(
      <RoverContextProvider value={withCtx()}>
        <OverviewPanel />
      </RoverContextProvider>
    );
    expect(screen.queryByTestId('joystick-pad')).toBeNull();
  });

  it('shows the no-cameras placeholder when none are advertised', () => {
    render(
      <RoverContextProvider value={withCtx({ cameraNames: [] })}>
        <OverviewPanel />
      </RoverContextProvider>
    );
    expect(screen.getByText(/no cameras advertised/i)).toBeInTheDocument();
  });

  it('renders advertised cameras regardless of name (e.g. auto-discovered camera-0/1)', () => {
    render(
      <RoverContextProvider value={withCtx({ cameraNames: ['camera-0', 'camera-1'], whepBase: 'http://rover' })}>
        <OverviewPanel />
      </RoverContextProvider>
    );
    // Before the fix these names were filtered against a hardcoded
    // ['chassis-front','gripper'] list and dropped, showing the placeholder.
    expect(screen.queryByText(/no cameras advertised/i)).toBeNull();
  });
});

describe('signalMetric', () => {
  it('N/A when no module', () => expect(signalMetric(null)).toEqual({ value: null, naReason: 'no connectivity module' }));
  it('N/A when offline', () => expect(signalMetric({ online: false })).toEqual({ value: null, naReason: 'uplink offline' }));
  it('N/A on ethernet', () => expect(signalMetric({ online: true })).toEqual({ value: null, naReason: 'ethernet uplink' }));
  it('bars when wifi/lte', () => expect(signalMetric({ online: true, signalBars: 3, signalBarsMax: 4 })).toEqual({ value: 3, unit: '/4' }));
});
