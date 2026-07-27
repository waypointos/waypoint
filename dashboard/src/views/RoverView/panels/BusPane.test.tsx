import { vi } from 'vitest';

const { busHandlers } = vi.hoisted(() => ({
  busHandlers: new Map<string, Set<(d: Uint8Array, s: string) => void>>(),
}));

vi.mock('@/state/nats', () => ({
  getBus: () => ({
    subscribe: (subject: string, h: (d: Uint8Array, s: string) => void) => {
      let set = busHandlers.get(subject);
      if (!set) busHandlers.set(subject, (set = new Set()));
      set.add(h);
      return () => set!.delete(h);
    },
    publish: () => {},
  }),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';
import { BusPane } from './BusPane';
import { RoverContextProvider, type RoverContextValue } from '../RoverContext';
import { PowerTelemetry } from '../../../../../protocol/gen/ts/messages/power_pb';
import { pbToBinary } from '@/state/protobuf';

function ctx(): RoverContextValue {
  return {
    id: 'demo', role: 'monitor', canControl: false,
    drive: null, power: null, sys: null, uplink: null, modules: [], image: null, motors: {},
    cameraNames: [], whepBase: '', roverAlerts: [],
    alertCounts: { info: 0, warning: 0, critical: 0 } as any,
    connection: { kind: 'offline' }, mode: 'unknown',
    recorder: null,
  };
}

function emit(concreteSubject: string, data: Uint8Array) {
  // The pane subscribes to the wildcard; the gateway delivers (data, concreteSubject).
  act(() => {
    for (const h of busHandlers.get('waypoint.demo.>') ?? []) h(data, concreteSubject);
  });
}

function renderPane() {
  return render(<RoverContextProvider value={ctx()}><BusPane /></RoverContextProvider>);
}

describe('BusPane', () => {
  beforeEach(() => busHandlers.clear());

  it('Monitor lists concrete subjects with their leaf names', () => {
    renderPane();
    emit('waypoint.demo.telemetry.power', pbToBinary(new PowerTelemetry({ busVoltageV: 12 })));
    expect(screen.getByText('telemetry.power')).toBeInTheDocument();
  });

  it('clicking a Monitor row drills into Raw filtered to that subject', () => {
    renderPane();
    emit('waypoint.demo.telemetry.power', pbToBinary(new PowerTelemetry({ busVoltageV: 12 })));
    fireEvent.click(screen.getByText('telemetry.power'));
    expect(screen.getByTestId('raw-breadcrumb')).toHaveTextContent('telemetry.power');
  });

  it('Raw decodes a known payload into fields', () => {
    renderPane();
    fireEvent.click(screen.getByText('Raw'));
    emit('waypoint.demo.telemetry.power', pbToBinary(new PowerTelemetry({ busVoltageV: 11.8 })));
    expect(screen.getByText(/busVoltageV 11\.8/)).toBeInTheDocument();
  });

  it('pauses ingestion', () => {
    renderPane();
    fireEvent.click(screen.getByText('PAUSE'));
    emit('waypoint.demo.heartbeat', new Uint8Array([1]));
    expect(screen.queryByText('heartbeat')).not.toBeInTheDocument();
  });

  it('Raw falls back to a hex preview for unknown subjects', () => {
    renderPane();
    fireEvent.click(screen.getByText('Raw'));
    emit('waypoint.demo.module.pm.stats', new Uint8Array([0x0a, 0x01]));
    expect(screen.getByText('2 B · 0a01')).toBeInTheDocument();
  });

  it('clearing the breadcrumb removes the drill-down filter', () => {
    renderPane();
    emit('waypoint.demo.telemetry.power', pbToBinary(new PowerTelemetry({ busVoltageV: 12 })));
    fireEvent.click(screen.getByText('telemetry.power'));
    expect(screen.getByTestId('raw-breadcrumb')).toBeInTheDocument();
    fireEvent.click(screen.getByText('✕'));
    expect(screen.queryByTestId('raw-breadcrumb')).not.toBeInTheDocument();
  });
});
