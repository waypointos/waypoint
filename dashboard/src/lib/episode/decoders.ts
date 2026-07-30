// dashboard/src/lib/episode/decoders.ts
//
// Schema-name keyed decoding for episode channels. Keys are protobuf full
// names as written by the recorder's MCAP schemas; unknown names fall through
// to the schemaless path (listed, not plotted).
import { pbFromBinary, pbToJson } from '../../state/protobuf';
import { DriveTelemetry, DriveCommand } from '../../../../protocol/gen/ts/messages/drive_pb';
import { ServoControl } from '../../../../protocol/gen/ts/messages/servo_pb';
import { MotorTelemetry } from '../../../../protocol/gen/ts/messages/motors_pb';
import { PowerTelemetry } from '../../../../protocol/gen/ts/messages/power_pb';
import {
  ArmState, ArmCommand, SensorReadings, BaseState, BaseCommand,
} from '../../../../protocol/gen/ts/messages/components_pb';
import { CompressedVideo } from '../../../../protocol/gen/ts/foxglove/compressed_video_pb';

const CLASSES = [
  DriveTelemetry, DriveCommand, ServoControl, MotorTelemetry, PowerTelemetry,
  ArmState, ArmCommand, SensorReadings, BaseState, BaseCommand, CompressedVideo,
] as const;

export const SCHEMA_REGISTRY: Record<string, unknown> = Object.fromEntries(
  CLASSES.map((c) => [(c as { typeName: string }).typeName, c]),
);

export const VIDEO_SCHEMA = 'foxglove.CompressedVideo';
const MOTOR_SCHEMA = 'waypoint.v1.MotorTelemetry';

export function decodeBySchema(schemaName: string, bytes: Uint8Array): Record<string, unknown> | null {
  const cls = SCHEMA_REGISTRY[schemaName];
  if (!cls) return null;
  try {
    // Without emitDefaultValues an implicit-presence scalar that reads exactly
    // 0 is dropped from the JSON, and a reported zero would plot as N/A.
    const json = pbToJson(pbFromBinary(cls, bytes), { emitDefaultValues: true });
    return json && typeof json === 'object' ? (json as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

export type Sample = { tNs: bigint; value: number | null };

/** Numeric leaves of one decoded message as (series id, sample) pairs. */
export function extractSeries(
  topic: string,
  schemaName: string,
  decoded: Record<string, unknown>,
  tNs: bigint,
): Array<{ id: string; sample: Sample }> {
  if (schemaName === 'waypoint.v1.SensorReadings') {
    const readings = (decoded.readings ?? []) as Array<Record<string, unknown>>;
    return readings
      .filter((r) => typeof r.name === 'string' && r.name !== '')
      .map((r) => ({
        id: `${topic}.${r.name as string}`,
        sample: { tNs, value: r.ok === true && typeof r.value === 'number' ? r.value : null },
      }));
  }
  // telemetry.motors carries one message per servo on one subject, so the
  // servo id has to key the series or every servo collapses into one line.
  const servoId = schemaName === MOTOR_SCHEMA && typeof decoded.id === 'number'
    ? String(decoded.id)
    : null;
  const prefix = servoId === null ? topic : `${topic}.${servoId}`;
  const out: Array<{ id: string; sample: Sample }> = [];
  const walk = (v: unknown, path: string): void => {
    // Log time is the plot axis; message stamps would plot as meaningless ramps.
    if (path === 'stamp' || path.startsWith('stamp.')) return;
    if (servoId !== null && path === 'id') return;
    if (typeof v === 'number') {
      out.push({ id: `${prefix}.${path}`, sample: { tNs, value: v } });
    } else if (Array.isArray(v)) {
      v.forEach((el, i) => walk(el, `${path}.${i}`));
    } else if (v && typeof v === 'object') {
      for (const [k, val] of Object.entries(v)) walk(val, path ? `${path}.${k}` : k);
    }
  };
  walk(decoded, '');
  return out;
}
