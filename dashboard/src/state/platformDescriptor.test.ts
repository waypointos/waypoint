import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  parseDescriptor,
  wheelSpeeds,
  platformOwnedBusIds,
  busIdForJoint,
} from '../../../protocol/platform/ts/descriptor';

const root = resolve(__dirname, '../../../protocol/platform');
const canonical = readFileSync(resolve(root, 'waypoint-rover.toml'), 'utf8');

type Golden = {
  joint_name_to_bus_id: Record<string, number>;
  platform_owned_bus_ids: number[];
  wheel_speeds: {
    vx_mps: number;
    wz_radps: number;
    expected: Record<string, number>;
  }[];
};

const fixtures = [
  {
    toml: readFileSync(resolve(root, 'waypoint-rover.toml'), 'utf8'),
    golden: JSON.parse(
      readFileSync(resolve(root, 'testdata/waypoint-rover.derived.golden.json'), 'utf8'),
    ) as Golden,
  },
  {
    toml: readFileSync(resolve(root, 'waypoint-bench.toml'), 'utf8'),
    golden: JSON.parse(
      readFileSync(resolve(root, 'testdata/waypoint-bench.derived.golden.json'), 'utf8'),
    ) as Golden,
  },
];

for (const { toml, golden } of fixtures) {
  const d = parseDescriptor(toml);

  describe(`platform descriptor TS conformance: ${d.platform.id}`, () => {
    it('resolves every joint name to its golden bus id', () => {
      for (const [name, id] of Object.entries(golden.joint_name_to_bus_id)) {
        expect(busIdForJoint(d, name), name).toBe(id);
      }
      expect(d.joints.length).toBe(Object.keys(golden.joint_name_to_bus_id).length);
    });

    it('derives the golden platform-owned deny-list', () => {
      expect([...platformOwnedBusIds(d)].sort((a, b) => a - b)).toEqual(
        golden.platform_owned_bus_ids,
      );
    });

    it('derives the golden wheel speeds', () => {
      for (const c of golden.wheel_speeds) {
        const got = wheelSpeeds(d, c.vx_mps, c.wz_radps);
        for (const [name, want] of Object.entries(c.expected)) {
          expect(got[name], `vx=${c.vx_mps} wz=${c.wz_radps} ${name}`).toBeCloseTo(want, 9);
        }
      }
    });
  });
}

describe('descriptor validation parity', () => {
  it('rejects an unknown vehicle_class', () => {
    const bad = canonical.replace('diff_drive_rover', 'hovercraft');
    expect(() => parseDescriptor(bad)).toThrow(/unknown vehicle_class/);
  });

  it('rejects fixed_base with kinematics', () => {
    const bench = readFileSync(resolve(root, 'waypoint-bench.toml'), 'utf8');
    const bad = `${bench}\n[kinematics]\nmodel = "diff_drive"\nwheel_radius_m = 0.07\ntrack_width_m = 0.3\n`;
    expect(() => parseDescriptor(bad)).toThrow(/fixed_base must not declare/);
  });

  it('rejects diff_drive_rover without kinematics', () => {
    const noKin = canonical.replace(/\[kinematics\][\s\S]*$/m, '');
    expect(() => parseDescriptor(noKin)).toThrow(/requires \[kinematics\]/);
  });
});
