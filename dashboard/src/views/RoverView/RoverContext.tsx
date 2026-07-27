// Per-rover state container. All subscriptions live in the shell that mounts
// this provider; panels consume it via useRoverContext().
import { createContext, useContext } from 'react';
import type { DriveTelemetry } from '../../../../protocol/gen/ts/messages/drive_pb';
import type { MotorTelemetry } from '../../../../protocol/gen/ts/messages/motors_pb';
import type { PowerTelemetry } from '../../../../protocol/gen/ts/messages/power_pb';
import type { SystemTelemetry } from '../../../../protocol/gen/ts/messages/system_pb';
import type { UplinkTelemetry } from '../../../../protocol/gen/ts/messages/uplink_pb';
import type { ModuleInfo } from '../../../../protocol/gen/ts/messages/modules_pb';
import type { ImageState } from '../../../../protocol/gen/ts/messages/image_pb';
import type { ActiveAlert, AlertCounts } from '@/state/useAlerts';
import type { Mode as DisplayMode } from '@/ui/telemetry/ModeIndicator';

export type ConnectionState =
  | { kind: 'offline' }
  | { kind: 'direct'; rttMs: number }
  | { kind: 'proxy';  rttMs: number };

export type Role = 'admin' | 'control' | 'monitor';

export type { DisplayMode };

// Episode recorder status, derived from the rover's event.recorder stream.
// null until the first RecorderEvent arrives.
export type RecorderStatus = {
  state: 'idle' | 'recording';
  episodeId: string;
  elapsedS: number;
  bytes: number;
  canStart: boolean;
  reason: string;
};

export type RoverContextValue = {
  id: string;
  role: Role;
  canControl: boolean;

  drive: DriveTelemetry | null;
  power: PowerTelemetry | null;
  sys:   SystemTelemetry | null;
  uplink: UplinkTelemetry | null;
  modules: ModuleInfo[];
  image: ImageState | null;
  motors: Record<number, MotorTelemetry>;

  cameraNames: string[];
  whepBase: string;

  roverAlerts: ActiveAlert[];
  alertCounts: AlertCounts;

  connection: ConnectionState;

  // Operational mode, sourced from core's event.mode stream. 'unknown' until
  // the first ModeEvent arrives and whenever the rover is offline. Treat
  // 'unknown' as "do not allow drive commands" — never assume manual.
  mode: DisplayMode;

  recorder: RecorderStatus | null;
};

const RoverContext = createContext<RoverContextValue | null>(null);

export const RoverContextProvider = RoverContext.Provider;

export function useRoverContext(): RoverContextValue {
  const v = useContext(RoverContext);
  if (!v) throw new Error('useRoverContext must be used inside RoverContextProvider');
  return v;
}
