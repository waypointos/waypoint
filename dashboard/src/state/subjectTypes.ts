// Maps a concrete NATS subject's leaf (the part after `waypoint.<id>.`) to its
// protobuf-es message class, derived from protocol/subjects.toml. Used by the
// Bus pane's Raw view to decode payloads. A drift test
// (subjectTypes.test.ts) keeps this in sync with subjects.toml.
import { pbFromBinary, pbToJson } from './protobuf';
import { DriveTelemetry, DriveCommand } from '../../../protocol/gen/ts/messages/drive_pb';
import { ServoControl, ServoSyncWrite } from '../../../protocol/gen/ts/messages/servo_pb';
import { GamepadSnapshot } from '../../../protocol/gen/ts/messages/gamepad_pb';
import { MotorTelemetry } from '../../../protocol/gen/ts/messages/motors_pb';
import { PowerTelemetry } from '../../../protocol/gen/ts/messages/power_pb';
import { SystemTelemetry } from '../../../protocol/gen/ts/messages/system_pb';
import { FaultEvent, ModeEvent, SystemEvent } from '../../../protocol/gen/ts/messages/events_pb';
import { AlertRaised, AlertAcknowledged, AlertResolved } from '../../../protocol/gen/ts/messages/alerts_pb';
import { Heartbeat } from '../../../protocol/gen/ts/messages/heartbeat_pb';
import { ImageState } from '../../../protocol/gen/ts/messages/image_pb';
import { CameraList } from '../../../protocol/gen/ts/messages/camera_pb';
import { AuditEvent } from '../../../protocol/gen/ts/messages/audit_pb';
import { ModuleSnapshot, DesiredModuleSet, ModuleReconcileEvent } from '../../../protocol/gen/ts/messages/modules_pb';
import { LogRecord } from '../../../protocol/gen/ts/messages/diag_pb';
import { UplinkTelemetry } from '../../../protocol/gen/ts/messages/uplink_pb';
import { ClockAnchor } from '../../../protocol/gen/ts/messages/clock_pb';
import { SimScenario } from '../../../protocol/gen/ts/messages/sim_pb';
import { PlatformInfo } from '../../../protocol/gen/ts/messages/platform_pb';
import { RecorderEvent } from '../../../protocol/gen/ts/messages/recorder_pb';

/** Leaf subject → generated message class. Keys mirror subjects.toml. */
export const REGISTRY: Record<string, unknown> = {
  'telemetry.drive': DriveTelemetry,
  'telemetry.motors': MotorTelemetry,
  'telemetry.power': PowerTelemetry,
  'telemetry.system': SystemTelemetry,
  'telemetry.uplink': UplinkTelemetry,
  'cmd.drive': DriveCommand,
  'cmd.servo': ServoControl,
  'cmd.gamepad': GamepadSnapshot,
  'cmd.servo_sync': ServoSyncWrite,
  'cmd.sim_scenario': SimScenario,
  'event.fault': FaultEvent,
  'event.mode': ModeEvent,
  'event.system': SystemEvent,
  'event.recorder': RecorderEvent,
  'alert.raised': AlertRaised,
  'alert.acknowledged': AlertAcknowledged,
  'alert.resolved': AlertResolved,
  'heartbeat': Heartbeat,
  'system.image': ImageState,
  'camera.list': CameraList,
  'infra.audit': AuditEvent,
  'infra.modules': ModuleSnapshot,
  'modules.desired': DesiredModuleSet,
  'modules.reconcile': ModuleReconcileEvent,
  'infra.clock': ClockAnchor,
  'infra.platform': PlatformInfo,
  'diag.log': LogRecord,
};

/** The subject leaf is everything after the `waypoint.<rover-id>.` prefix. */
export function subjectLeaf(subject: string): string {
  return subject.split('.').slice(2).join('.');
}

/** Decode payload bytes for a known subject into a JSON object, or null if the
 * subject is unknown or the bytes don't decode. Never throws. */
export function decodeBySubject(subject: string, bytes: Uint8Array): Record<string, unknown> | null {
  const cls = REGISTRY[subjectLeaf(subject)];
  if (!cls) return null;
  try {
    const json = pbToJson(pbFromBinary(cls, bytes));
    return json && typeof json === 'object' ? (json as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

/** Flatten a decoded object into a compact `key value  key value` string. */
export function formatFields(obj: Record<string, unknown>): string {
  return Object.entries(obj)
    .map(([k, v]) => `${k} ${fmtVal(v)}`)
    .join('  ');
}

function fmtVal(v: unknown): string {
  if (v === null || v === undefined) return '—';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

/** Fallback preview for undecodable payloads: byte count + hex prefix. */
export function hexPreview(bytes: Uint8Array, maxBytes = 8): string {
  if (bytes.length === 0) return '0 B';
  let hex = '';
  for (let i = 0; i < Math.min(bytes.length, maxBytes); i++) {
    hex += bytes[i].toString(16).padStart(2, '0');
  }
  return `${bytes.length} B · ${hex}`;
}
