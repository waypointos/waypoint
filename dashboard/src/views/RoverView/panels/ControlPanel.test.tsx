import { vi } from 'vitest';

vi.mock('@/state/useDriveCmd', () => ({ useDriveCmd: () => {} }));
const requestMode = vi.fn();
const requestEstop = vi.fn();
const requestRecover = vi.fn();
vi.mock('@/state/useModeCommand', () => ({
  useModeCommand: () => ({ pending: null, requestMode, requestEstop, requestRecover }),
}));

// Mock the bus so usePlatform's infra.platform subscription is controllable and
// never opens a real connection. Handlers are kept so a test can publish.
const subscribers = new Map<string, (b: Uint8Array, subject: string) => void>();
vi.mock('@/state/nats', () => ({
  getBus: () => ({
    subscribe: (subject: string, h: (b: Uint8Array, subject: string) => void) => {
      subscribers.set(subject, h);
      return () => subscribers.delete(subject);
    },
    publish: () => {},
  }),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { PlatformInfo } from '../../../../../protocol/gen/ts/messages/platform_pb';
import { pbToBinary } from '@/state/protobuf';
import { ControlPanel } from './ControlPanel';
import { RoverContextProvider, type RoverContextValue } from '../RoverContext';

// Push a PlatformInfo through the mocked bus to the platform subscription.
function publishPlatform(roverId: string, info: PlatformInfo) {
  const h = subscribers.get(`waypoint.${roverId}.infra.platform`);
  act(() => h?.(pbToBinary(info), `waypoint.${roverId}.infra.platform`));
}

const benchInfo = new PlatformInfo({
  platformId: 'waypoint-bench',
  vehicleClass: 'fixed_base',
  schema: 1,
  joints: [
    { name: 'arm_1', busId: 1, type: 'revolute', ownership: 'module', invert: false },
    { name: 'gripper', busId: 6, type: 'gripper', ownership: 'module', invert: false },
  ],
});

function withCtx(overrides: Partial<RoverContextValue> = {}): RoverContextValue {
  return {
    id: 'demo', role: 'control', canControl: true,
    drive: null, power: null, sys: null, uplink: null, modules: [], image: null, motors: {},
    cameraNames: [], whepBase: '',
    roverAlerts: [], alertCounts: { info: 0, warning: 0, critical: 0 } as any,
    connection: { kind: 'offline' }, mode: 'unknown',
    recorder: null,
    ...overrides,
  };
}

function renderPanel(overrides: Partial<RoverContextValue> = {}) {
  return render(
    <MemoryRouter>
      <RoverContextProvider value={withCtx(overrides)}>
        <ControlPanel />
      </RoverContextProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  requestMode.mockClear();
  requestEstop.mockClear();
  requestRecover.mockClear();
  subscribers.clear();
});

describe('ControlPanel', () => {
  it('renders drive controls when role can control', () => {
    renderPanel();
    expect(screen.getByText('Throttle')).toBeInTheDocument();
    expect(screen.queryByText('READ-ONLY')).toBeNull();
  });

  it('shows READ-ONLY and hides the joystick for monitor role', () => {
    renderPanel({ role: 'monitor', canControl: false });
    expect(screen.getByText('READ-ONLY')).toBeInTheDocument();
    expect(screen.queryByText('Throttle')).toBeNull();
  });

  it('requests a mode change when an enabled mode button is clicked', () => {
    renderPanel({ connection: { kind: 'direct', rttMs: 10 }, mode: 'safe' });
    screen.getByRole('button', { name: 'Manual' }).click();
    expect(requestMode).toHaveBeenCalledWith('manual');
  });

  it('engages the E-stop', () => {
    renderPanel({ connection: { kind: 'direct', rttMs: 10 }, mode: 'manual' });
    screen.getByRole('button', { name: /emergency stop/i }).click();
    expect(requestEstop).toHaveBeenCalled();
  });

  it('clears the E-stop when estopped', () => {
    renderPanel({ connection: { kind: 'direct', rttMs: 10 }, mode: 'estop' });
    screen.getByRole('button', { name: /clear e-stop/i }).click();
    expect(requestRecover).toHaveBeenCalled();
  });

  it('links to the teleop console', () => {
    renderPanel({ connection: { kind: 'direct', rttMs: 10 }, mode: 'manual' });
    const link = screen.getByRole('link', { name: /teleop/i });
    expect(link.getAttribute('href')).toBe('/rover/demo/teleop');
  });

  it('renders the joystick when the platform has drive', () => {
    // No infra.platform message yet: the fallback is the baked rover (has drive).
    renderPanel({ connection: { kind: 'direct', rttMs: 10 }, mode: 'manual' });
    expect(screen.getByText('Throttle')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /teleop/i })).toBeInTheDocument();
  });

  it('hides drive controls on a fixed_base platform', () => {
    renderPanel({ connection: { kind: 'direct', rttMs: 10 }, mode: 'manual' });
    publishPlatform('demo', benchInfo);
    // Drive-coupled UI is hidden, not rendered as N/A.
    expect(screen.queryByText('Throttle')).toBeNull();
    expect(screen.queryByRole('link', { name: /teleop/i })).toBeNull();
    expect(screen.queryByText('DRIVE')).toBeNull();
    // E-stop and mode controls are platform-wide and remain.
    expect(screen.getByRole('button', { name: /emergency stop/i })).toBeInTheDocument();
  });
});
