import { describe, expect, it } from 'vitest';
import { SensorReadings, SensorReading } from '../../../../protocol/gen/ts/messages/components_pb';
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
      motors: [{ id: 1, speed: 0.5, faulted: false, note: 'x' }],
    };
    const out = extractSeries('telemetry.motors', 'waypoint.v1.MotorTelemetry', decoded, 7n);
    const ids = out.map((s) => s.id);
    expect(ids).toContain('telemetry.motors.motors.0.speed');
    expect(ids).toContain('telemetry.motors.motors.0.id');
    expect(ids.some((i) => i.includes('stamp'))).toBe(false);
    expect(ids.some((i) => i.includes('note'))).toBe(false);
  });
});
