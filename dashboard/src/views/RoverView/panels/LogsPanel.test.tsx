import { vi } from 'vitest';

vi.mock('@/state/nats', () => ({
  getBus: () => ({ subscribe: () => () => {}, publish: () => {} }),
}));

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LogsPanel } from './LogsPanel';
import { RoverContextProvider, type RoverContextValue } from '../RoverContext';

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

describe('LogsPanel', () => {
  it('renders the Logs stream and the Bus pane', () => {
    render(<RoverContextProvider value={ctx()}><LogsPanel /></RoverContextProvider>);
    expect(screen.getByTestId('logs-stream')).toBeInTheDocument();
    expect(screen.getByTestId('bus-pane')).toBeInTheDocument();
    expect(screen.getByText('Logs')).toBeInTheDocument();
    expect(screen.getByText('Bus')).toBeInTheDocument();
  });
});
