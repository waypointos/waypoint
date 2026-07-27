import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SystemPanel } from './SystemPanel';
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

describe('SystemPanel', () => {
  it('renders all four host cells', () => {
    render(
      <RoverContextProvider value={withCtx()}>
        <SystemPanel />
      </RoverContextProvider>
    );
    expect(screen.getByText('CPU')).toBeInTheDocument();
    expect(screen.getByText('Memory')).toBeInTheDocument();
    expect(screen.getByText('Temp')).toBeInTheDocument();
    expect(screen.getByText('Link RTT')).toBeInTheDocument();
  });

  it('shows image fields from ImageState when present', () => {
    render(
      <RoverContextProvider value={withCtx({
        image: { version: 'v0.4.1', partition: 'A', variant: 'prod', bootcount: 17, builtAt: '2026-05-12' } as any,
      })}>
        <SystemPanel />
      </RoverContextProvider>
    );
    expect(screen.getByText('v0.4.1')).toBeInTheDocument();
    expect(screen.getByText('A')).toBeInTheDocument();
    expect(screen.getByText('prod')).toBeInTheDocument();
    expect(screen.getByText('17')).toBeInTheDocument();
  });

  it('falls back to SystemTelemetry imageVersion/imagePartition when ImageState is absent', () => {
    render(
      <RoverContextProvider value={withCtx({
        sys: { imageVersion: 'v0.3.9', imagePartition: 'B' } as any,
      })}>
        <SystemPanel />
      </RoverContextProvider>
    );
    expect(screen.getByText('v0.3.9')).toBeInTheDocument();
    expect(screen.getByText('B')).toBeInTheDocument();
  });

  it('formats uptime in days/hours/minutes', () => {
    render(
      <RoverContextProvider value={withCtx({
        sys: { uptimeSeconds: 90061 } as any, // 1d 1h 1m
      })}>
        <SystemPanel />
      </RoverContextProvider>
    );
    expect(screen.getByText('1d 1h 1m')).toBeInTheDocument();
  });
});
