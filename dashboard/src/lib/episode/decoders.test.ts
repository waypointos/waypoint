import { describe, expect, it } from 'vitest';
import {
  SensorReadings, SensorReading, ArmState, ArmJoint,
} from '../../../../protocol/gen/ts/messages/components_pb';
import { MotorTelemetry } from '../../../../protocol/gen/ts/messages/motors_pb';
import { pbToBinary } from '../../state/protobuf';
import { decodeBySchema, extractSeries } from './decoders';

function sensorBytes(): Uint8Array {
  const msg = new SensorReadings({
    readings: [
      new SensorReading({ name: 'cell_a_g', value: 100, unit: 'g', ok: true }),
      new SensorReading({ name: 'cell_b_g', unit: 'g', ok: false }),
    ],
  });
  return pbToBinary(msg);
}

describe('decodeBySchema', () => {
  it('decodes a registered schema', () => {
    const obj = decodeBySchema('waypoint.v1.SensorReadings', sensorBytes());
    expect(obj).not.toBeNull();
  });

  it('returns null for an unknown schema (schemaless fallthrough)', () => {
    expect(decodeBySchema('waypoint.v1.NoSuchThing', new Uint8Array([1]))).toBeNull();
  });

  it('returns null on undecodable bytes instead of throwing', () => {
    expect(decodeBySchema('waypoint.v1.SensorReadings', new Uint8Array([0xff, 0xff, 0xff]))).toBeNull();
  });
});

describe('extractSeries', () => {
  it('maps SensorReadings entries to reading-name series and null for ok=false', () => {
    const obj = decodeBySchema('waypoint.v1.SensorReadings', sensorBytes())!;
    const out = extractSeries('module.drill.sensor.state', 'waypoint.v1.SensorReadings', obj, 5n);
    const byId = Object.fromEntries(out.map((s) => [s.id, s.sample]));
    expect(byId['module.drill.sensor.state.cell_a_g']).toEqual({ tNs: 5n, value: 100 });
    expect(byId['module.drill.sensor.state.cell_b_g']).toEqual({ tNs: 5n, value: null });
  });

  it('flattens generic numeric fields with dotted paths and skips stamp', () => {
    const decoded = {
      stamp: { unixNanos: '123' },
      joints: [{ name: 'shoulder', positionRad: 0.5, calibrated: true }],
    };
    const out = extractSeries('module.arm.arm.state', 'waypoint.v1.ArmState', decoded, 7n);
    const ids = out.map((s) => s.id);
    expect(ids).toContain('module.arm.arm.state.joints.0.positionRad');
    expect(ids.some((i) => i.includes('stamp'))).toBe(false);
    expect(ids.some((i) => i.includes('name'))).toBe(false);
  });

  it('keys motor series by servo id so servos do not collapse into one series', () => {
    const bytesFor = (id: number, v: number) =>
      pbToBinary(new MotorTelemetry({ id, velocityRadps: v }));
    const first = decodeBySchema('waypoint.v1.MotorTelemetry', bytesFor(7, 1.5))!;
    const second = decodeBySchema('waypoint.v1.MotorTelemetry', bytesFor(8, -1.5))!;
    const ids = (d: Record<string, unknown>) =>
      extractSeries('telemetry.motors', 'waypoint.v1.MotorTelemetry', d, 7n).map((s) => s.id);
    expect(ids(first)).toContain('telemetry.motors.7.velocityRadps');
    expect(ids(second)).toContain('telemetry.motors.8.velocityRadps');
    // The id is the series key, not a plottable measurement.
    expect(ids(first)).not.toContain('telemetry.motors.7.id');
  });

  it('keeps a genuinely reported zero instead of dropping it to N/A', () => {
    const bytes = pbToBinary(new ArmState({
      joints: [new ArmJoint({ name: 'shoulder', positionRad: 0 })],
    }));
    const decoded = decodeBySchema('waypoint.v1.ArmState', bytes)!;
    const out = extractSeries('module.arm.arm.state', 'waypoint.v1.ArmState', decoded, 9n);
    const byId = Object.fromEntries(out.map((s) => [s.id, s.sample]));
    expect(byId['module.arm.arm.state.joints.0.positionRad']).toEqual({ tNs: 9n, value: 0 });
  });
});
