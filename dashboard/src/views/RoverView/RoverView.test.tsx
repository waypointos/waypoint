import { vi } from 'vitest';

const subscribers = new Map<string, (b: Uint8Array, subject?: string) => void>();
vi.mock('@/state/nats', () => ({
  getBus: () => ({
    subscribe: (subject: string, h: (b: Uint8Array, subject?: string) => void) => {
      subscribers.set(subject, h);
      return () => subscribers.delete(subject);
    },
    publish: () => {},
  }),
}));
vi.mock('@/state/mode',      () => ({ useMe:      () => ({ mode: 'local', isAdmin: true }) }));
vi.mock('@/state/useFleet',  () => ({ useFleet:   () => ({ rovers: [] }) }));
vi.mock('@/state/useAlerts', () => ({
  useAlerts: () => ({ alerts: [], counts: { info: 0, warning: 0, critical: 0 } }),
  acknowledgeAlert: () => {},
  resolveAlert: () => {},
}));
vi.mock('@/state/useDriveCmd', () => ({ useDriveCmd: () => {} }));

import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { RoverView } from './RoverView';
import { ModuleSnapshot, ModuleInfo, ModuleUI, ModuleUI_Kind } from '../../../../protocol/gen/ts/messages/modules_pb';
import { pbToBinary } from '@/state/protobuf';

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/rover/:id"        element={<RoverView />} />
        <Route path="/rover/:id/:tab"   element={<RoverView />} />
      </Routes>
    </MemoryRouter>
  );
}

describe('RoverView shell', () => {
  it('redirects /rover/:id to /rover/:id/overview', () => {
    renderAt('/rover/demo');
    expect(screen.getByTestId('panel-overview')).toBeVisible();
  });

  it('renders all four panels in the DOM; only active is visible', () => {
    renderAt('/rover/demo/control');
    expect(screen.getByTestId('panel-control')).toBeVisible();
    const overview = screen.getByTestId('panel-overview');
    expect(overview).toBeInTheDocument();
    expect(overview).not.toBeVisible();
  });

  it('redirects unknown tab segments to overview', () => {
    renderAt('/rover/demo/gibberish');
    expect(screen.getByTestId('panel-overview')).toBeVisible();
  });

  it('renders a module tab when the module is healthy and declares a UI', async () => {
    renderAt('/rover/demo/overview');
    const snapshot = new ModuleSnapshot({
      modules: [
        new ModuleInfo({
          id: 'umr',
          label: 'Connectivity',
          version: '0.1',
          tabId: 'm-umr',
          healthy: true,
          ui: new ModuleUI({ kind: ModuleUI_Kind.STATIC, tabId: 'm-umr', lanOnly: false }),
        }),
      ],
    });
    subscribers.get('waypoint.demo.infra.modules')?.(pbToBinary(snapshot));
    await waitFor(() => expect(screen.getByText('CONNECTIVITY')).toBeInTheDocument());
  });

  it('omits module tabs when no modules are reported', () => {
    renderAt('/rover/demo/overview');
    expect(screen.queryByText('CONNECTIVITY')).not.toBeInTheDocument();
  });
});
