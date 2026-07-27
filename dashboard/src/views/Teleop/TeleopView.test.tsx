import { vi } from 'vitest';

vi.mock('@/state/useDriveCmd', () => ({ useDriveCmd: () => {} }));
vi.mock('@/state/useGamepadPublish', () => ({ useGamepadPublish: () => {} }));
vi.mock('@/state/mode', () => ({ useMe: () => ({ mode: 'local' }) }));
const requestMode = vi.fn();
const requestEstop = vi.fn();
vi.mock('@/state/useModeCommand', () => ({
  useModeCommand: () => ({ pending: null, requestMode, requestEstop, requestRecover: vi.fn() }),
}));
vi.mock('@/ui/telemetry/CameraStage', () => ({ CameraStage: () => <div data-testid="camera-stage" /> }));
// Stub the module-window host so its dynamic-import machinery never runs in jsdom.
vi.mock('./TeleopWindowHost', () => ({
  TeleopWindowHost: ({ windows }: { windows: unknown[] }) => (
    <div data-testid="window-host" data-count={windows.length} />
  ),
}));

// Pinned platform shape so the HUD wheel cluster is deterministic and
// usePlatform never touches the NATS bus in jsdom.
vi.mock('@/state/platform', () => ({
  usePlatform: () => ({
    platformId: 'waypoint-rover',
    vehicleClass: 'diff_drive_rover',
    hasDrive: true,
    kinematics: {
      wheelRadiusM: 0.07425,
      trackWidthM: 0.3,
      wheels: {
        front_left: 'wheel_front_left', front_right: 'wheel_front_right',
        back_left: 'wheel_back_left', back_right: 'wheel_back_right',
      },
    },
    joints: [
      { name: 'wheel_back_left',   busId: 7,  type: 'wheel', ownership: 'platform', invert: true },
      { name: 'wheel_back_right',  busId: 8,  type: 'wheel', ownership: 'platform', invert: false },
      { name: 'wheel_front_right', busId: 9,  type: 'wheel', ownership: 'platform', invert: false },
      { name: 'wheel_front_left',  busId: 10, type: 'wheel', ownership: 'platform', invert: true },
    ],
  }),
}));

import { useRoverData } from '@/views/RoverView/useRoverData';
vi.mock('@/views/RoverView/useRoverData', () => ({ useRoverData: vi.fn() }));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { TeleopView } from './TeleopView';
import type { RoverContextValue } from '@/views/RoverView/RoverContext';

function ctx(overrides: Partial<RoverContextValue> = {}): RoverContextValue {
  return {
    id: 'demo', role: 'control', canControl: true,
    drive: null, power: null, sys: null, uplink: null, modules: [], image: null, motors: {},
    cameraNames: ['front'], whepBase: '',
    roverAlerts: [], alertCounts: { info: 0, warning: 0, critical: 0 } as any,
    connection: { kind: 'direct', rttMs: 10 }, mode: 'manual',
    recorder: null,
    ...overrides,
  };
}

function renderAt(value: RoverContextValue) {
  (useRoverData as any).mockReturnValue(value);
  return render(
    <MemoryRouter initialEntries={['/rover/demo/teleop']}>
      <Routes><Route path="/rover/:id/teleop" element={<TeleopView />} /></Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => { requestMode.mockClear(); requestEstop.mockClear(); });

describe('TeleopView', () => {
  it('renders the camera stage and HUD mode control', () => {
    renderAt(ctx());
    expect(screen.getByTestId('camera-stage')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Manual' })).toBeInTheDocument();
  });

  it('exposes an Exit back to the control tab', () => {
    renderAt(ctx());
    expect(screen.getByRole('link', { name: /exit/i }).getAttribute('href')).toBe('/rover/demo/control');
  });

  it('engages the E-stop from the HUD', () => {
    renderAt(ctx());
    screen.getByRole('button', { name: /emergency stop/i }).click();
    expect(requestEstop).toHaveBeenCalled();
  });

  it('switches mode from the HUD', () => {
    renderAt(ctx({ mode: 'safe' }));
    screen.getByRole('button', { name: 'Manual' }).click();
    expect(requestMode).toHaveBeenCalledWith('manual');
  });

  it('hosts module teleop windows (no hardcoded arm overlay)', () => {
    renderAt(ctx());
    expect(screen.getByTestId('window-host')).toBeInTheDocument();
    expect(screen.queryByTestId('arm-overlay')).toBeNull();
  });

  it('renders a wheel bar per drive wheel from the platform shape', () => {
    const { container } = renderAt(ctx());
    const wheels = container.querySelectorAll('[data-wheel]');
    expect(Array.from(wheels).map((w) => w.getAttribute('data-wheel'))).toEqual(['FL', 'FR', 'BL', 'BR']);
  });

  it('renders telemetry rail N/A states with reasons when telemetry is absent', () => {
    renderAt(ctx());
    expect(screen.getAllByText('no power telemetry').length).toBeGreaterThan(0);
    expect(screen.getByText('no imu fitted')).toBeInTheDocument();
  });

  it('shows the e-stop banner while estopped', () => {
    renderAt(ctx({ mode: 'estop' }));
    expect(screen.getByRole('alert').textContent).toMatch(/e-stop engaged/i);
  });

  it('shows subsystem pills', () => {
    const { container } = renderAt(ctx());
    expect(container.querySelector('[data-status-pills]')).not.toBeNull();
  });
});
