import { describe, expect, it } from 'vitest';
import { PlatformInfo } from '../../../protocol/gen/ts/messages/platform_pb';
import {
  FALLBACK_PLATFORM,
  fromInfo,
  leftWheelBusIds,
  rightWheelBusIds,
  wheelBusIds,
} from './platform';

const benchInfo = new PlatformInfo({
  platformId: 'waypoint-bench',
  vehicleClass: 'fixed_base',
  schema: 1,
  joints: [
    { name: 'arm_1', busId: 1, type: 'revolute', ownership: 'module', invert: false },
    { name: 'gripper', busId: 6, type: 'gripper', ownership: 'module', invert: false },
  ],
});

describe('platform store', () => {
  it('fallback is the baked rover with drive', () => {
    expect(FALLBACK_PLATFORM.platformId).toBe('waypoint-rover');
    expect(FALLBACK_PLATFORM.hasDrive).toBe(true);
    expect(FALLBACK_PLATFORM.kinematics?.wheelRadiusM).toBeCloseTo(0.07425, 9);
    expect([...leftWheelBusIds(FALLBACK_PLATFORM)].sort()).toEqual([10, 7]);
    expect([...rightWheelBusIds(FALLBACK_PLATFORM)].sort()).toEqual([8, 9]);
  });

  it('maps a fixed_base PlatformInfo to a drive-less shape', () => {
    const p = fromInfo(benchInfo);
    expect(p.platformId).toBe('waypoint-bench');
    expect(p.hasDrive).toBe(false);
    expect(p.kinematics).toBeUndefined();
    expect(p.joints.map((j) => j.name)).toEqual(['arm_1', 'gripper']);
    expect(wheelBusIds(p)).toEqual([]);
  });
});
