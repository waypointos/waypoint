import { describe, it, expect } from 'vitest';
import { applySpeedScale, PRESET_FACTOR } from './speed';

describe('applySpeedScale', () => {
  it('scales by preset factor at full cap', () => {
    const out = applySpeedScale({ vx: 1, yaw: -1 }, 'cruise', 1);
    expect(out.vx).toBeCloseTo(PRESET_FACTOR.cruise);
    expect(out.yaw).toBeCloseTo(-PRESET_FACTOR.cruise);
  });

  it('multiplies preset by cap', () => {
    const out = applySpeedScale({ vx: 1, yaw: 0 }, 'turbo', 0.5);
    expect(out.vx).toBeCloseTo(0.5);
  });

  it('clamps cap to 0..1', () => {
    expect(applySpeedScale({ vx: 1, yaw: 0 }, 'turbo', 5).vx).toBeCloseTo(1);
    expect(applySpeedScale({ vx: 1, yaw: 0 }, 'turbo', -1).vx).toBeCloseTo(0);
  });
});
