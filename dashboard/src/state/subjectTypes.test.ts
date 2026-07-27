// @vitest-environment node
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import {
  REGISTRY,
  subjectLeaf,
  decodeBySubject,
  formatFields,
  hexPreview,
} from './subjectTypes';
import { PowerTelemetry } from '../../../protocol/gen/ts/messages/power_pb';
import { pbToBinary } from './protobuf';

describe('subjectLeaf', () => {
  it('strips the waypoint.<id>. prefix', () => {
    expect(subjectLeaf('waypoint.dev-rover.telemetry.power')).toBe('telemetry.power');
    expect(subjectLeaf('waypoint.dev-rover.heartbeat')).toBe('heartbeat');
    expect(subjectLeaf('waypoint.dev-rover.module.pm.stats')).toBe('module.pm.stats');
  });
});

describe('REGISTRY drift vs subjects.toml', () => {
  const toml = readFileSync(
    new URL('../../../protocol/subjects.toml', import.meta.url),
    'utf8',
  );
  const expected = new Map<string, string>();
  for (const line of toml.split('\n')) {
    const m = line.match(/^"([^"]+)"\s*=\s*"([^"]+)"/);
    if (!m) continue;
    const [, leaf, type] = m;
    if (type.includes('->')) continue;        // RPC request/response pair
    if (/reserved/i.test(line)) continue;       // reserved / not-yet-live
    if (leaf.includes('<')) continue;           // placeholder token
    expected.set(leaf, type);
  }

  it('covers every live single-message subject and maps to the right type', () => {
    for (const [leaf, type] of expected) {
      const cls = REGISTRY[leaf];
      expect(cls, `registry missing subject "${leaf}" — add it to REGISTRY in subjectTypes.ts`).toBeDefined();
      const shortName = String((cls as { typeName: string }).typeName).split('.').pop();
      expect(shortName, `subject "${leaf}" mapped to wrong type in subjectTypes.ts`).toBe(type);
    }
  });

  it('has no registry entries absent from subjects.toml', () => {
    for (const leaf of Object.keys(REGISTRY)) {
      expect(expected.has(leaf), `registry has stale subject "${leaf}" — remove it from REGISTRY in subjectTypes.ts`).toBe(true);
    }
  });
});

describe('decodeBySubject', () => {
  it('decodes a known subject to a JSON object', () => {
    const bytes = pbToBinary(new PowerTelemetry({ busVoltageV: 11.8 }));
    const json = decodeBySubject('waypoint.dev-rover.telemetry.power', bytes) as Record<string, unknown>;
    expect(json).not.toBeNull();
    expect(json.busVoltageV).toBeCloseTo(11.8);
  });

  it('returns null for an unknown subject', () => {
    expect(decodeBySubject('waypoint.dev-rover.module.pm.stats', new Uint8Array([1, 2]))).toBeNull();
  });

  it('returns null (no throw) on malformed bytes for a known subject', () => {
    expect(decodeBySubject('waypoint.dev-rover.telemetry.power', new Uint8Array([0xff, 0xff, 0xff]))).toBeNull();
  });
});

describe('formatFields', () => {
  it('renders top-level entries as "key value" pairs', () => {
    expect(formatFields({ roll: -2.1, yaw: 88.7 })).toBe('roll -2.1  yaw 88.7');
  });
  it('shows em dash for null/undefined', () => {
    expect(formatFields({ source: null })).toBe('source —');
  });
});

describe('hexPreview', () => {
  it('renders byte count and a hex prefix', () => {
    expect(hexPreview(new Uint8Array([0x0a, 0xff, 0x01]))).toBe('3 B · 0aff01');
  });
  it('omits the separator for empty payloads', () => {
    expect(hexPreview(new Uint8Array([]))).toBe('0 B');
  });
});
