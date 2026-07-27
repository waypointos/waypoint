// dashboard/src/views/RoverView/useRoverData.ts
//
// All per-rover subscriptions and derived state, in one hook so both the tabbed
// RoverView and the full-screen TeleopView build an identical RoverContextValue.
import { useEffect, useMemo, useState } from 'react';
import { getBus } from '@/state/nats';
import { pbFromBinary } from '@/state/protobuf';
import { useTelemetry } from '@/state/useTelemetry';
import { useFleet } from '@/state/useFleet';
import { useMe } from '@/state/mode';
import { useAlerts } from '@/state/useAlerts';

import { DriveTelemetry } from '../../../../protocol/gen/ts/messages/drive_pb';
import { MotorTelemetry } from '../../../../protocol/gen/ts/messages/motors_pb';
import { PowerTelemetry } from '../../../../protocol/gen/ts/messages/power_pb';
import { SystemTelemetry } from '../../../../protocol/gen/ts/messages/system_pb';
import { UplinkTelemetry } from '../../../../protocol/gen/ts/messages/uplink_pb';
import { ModuleSnapshot } from '../../../../protocol/gen/ts/messages/modules_pb';
import { CameraList } from '../../../../protocol/gen/ts/messages/camera_pb';
import { ImageState } from '../../../../protocol/gen/ts/messages/image_pb';
import { ModeEvent } from '../../../../protocol/gen/ts/messages/events_pb';
import { Mode } from '../../../../protocol/gen/ts/messages/common_pb';
import { RecorderEvent, RecorderState } from '../../../../protocol/gen/ts/messages/recorder_pb';

import type { ConnectionState, DisplayMode, RecorderStatus, Role, RoverContextValue } from './RoverContext';

export type { RecorderStatus };

export function displayModeFromProto(m: Mode): DisplayMode {
  switch (m) {
    case Mode.MANUAL:     return 'manual';
    case Mode.SAFE:       return 'safe';
    case Mode.AUTONOMOUS: return 'autonomous';
    case Mode.ESTOP:      return 'estop';
    default:              return 'unknown';
  }
}

export function useRoverData(id: string): RoverContextValue {
  const me = useMe();
  const { rovers } = useFleet();

  const role: Role =
    me?.mode === 'local'
      ? 'admin'
      : ((rovers.find((r) => r.id === id)?.role as Role | undefined) ?? 'monitor');
  const canControl = role !== 'monitor';

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const drive = useTelemetry(`waypoint.${id}.telemetry.drive`,  (b) => pbFromBinary<DriveTelemetry>(DriveTelemetry, b));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const power = useTelemetry(`waypoint.${id}.telemetry.power`,  (b) => pbFromBinary<PowerTelemetry>(PowerTelemetry, b));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const sys   = useTelemetry(`waypoint.${id}.telemetry.system`, (b) => pbFromBinary<SystemTelemetry>(SystemTelemetry, b));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const uplink = useTelemetry(`waypoint.${id}.telemetry.uplink`, (b) => pbFromBinary<UplinkTelemetry>(UplinkTelemetry, b));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const snapshot = useTelemetry(`waypoint.${id}.infra.modules`, (b) => ModuleSnapshot.fromBinary(b));
  const modules = snapshot?.modules ?? [];
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const image = useTelemetry(`waypoint.${id}.infra.system.image`, (b) => pbFromBinary<ImageState>(ImageState, b));

  const { alerts: allAlerts, counts: alertCounts } = useAlerts();
  const roverAlerts = useMemo(() => allAlerts.filter((a) => a.roverId === id), [allAlerts, id]);

  const [motors, setMotors] = useState<Record<number, MotorTelemetry>>({});
  useEffect(() => {
    setMotors({});
    const off = getBus().subscribe(`waypoint.${id}.telemetry.motors`, (bytes) => {
      try {
        const m = pbFromBinary<MotorTelemetry>(MotorTelemetry, bytes);
        setMotors((prev) => ({ ...prev, [m.id]: m }));
      } catch { /* ignore decode errors */ }
    });
    return off;
  }, [id]);

  const [cameraNames, setCameraNames] = useState<string[]>([]);
  useEffect(() => {
    setCameraNames([]);
    const off = getBus().subscribe(`waypoint.${id}.infra.camera.list`, (bytes) => {
      try {
        const list = pbFromBinary<CameraList>(CameraList, bytes);
        setCameraNames(list.cameras.map((c) => c.name));
      } catch { /* ignore decode errors */ }
    });
    return off;
  }, [id]);

  const [coreMode, setCoreMode] = useState<DisplayMode>('unknown');
  useEffect(() => {
    setCoreMode('unknown');
    const off = getBus().subscribe(`waypoint.${id}.event.mode`, (bytes) => {
      try {
        const ev = pbFromBinary<ModeEvent>(ModeEvent, bytes);
        setCoreMode(displayModeFromProto(ev.to));
      } catch { /* ignore decode errors */ }
    });
    return off;
  }, [id]);

  const [recorder, setRecorder] = useState<RecorderStatus | null>(null);
  useEffect(() => {
    setRecorder(null);
    const off = getBus().subscribe(`waypoint.${id}.event.recorder`, (bytes) => {
      try {
        const ev = pbFromBinary<RecorderEvent>(RecorderEvent, bytes);
        setRecorder({
          state: ev.state === RecorderState.RECORDING ? 'recording' : 'idle',
          episodeId: ev.episodeId,
          elapsedS: ev.elapsedS,
          bytes: Number(ev.bytesWritten),
          canStart: ev.canStart,
          reason: ev.reason,
        });
      } catch { /* ignore decode errors */ }
    });
    return off;
  }, [id]);

  const whepBase = me?.mode === 'local' ? '' : `/rover/${id}`;

  const connection: ConnectionState = !sys
    ? { kind: 'offline' }
    : me?.mode === 'proxy'
      ? { kind: 'proxy',  rttMs: Math.round(sys.linkRttMs ?? 0) }
      : { kind: 'direct', rttMs: Math.round(sys.linkRttMs ?? 0) };

  const mode: DisplayMode = connection.kind === 'offline' ? 'unknown' : coreMode;

  return {
    id, role, canControl,
    drive, power, sys, uplink, modules, image, motors,
    cameraNames, whepBase,
    roverAlerts, alertCounts,
    connection, mode,
    recorder,
  };
}
