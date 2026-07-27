// Platform descriptor loader for TypeScript consumers. Parsing and derived
// values only; full validation is the Go package's job (the descriptor a
// dashboard sees has already been validated on the rover). Kept in golden-
// fixture conformance with the Go implementation.
import { parse } from 'smol-toml';

export interface Joint {
  name: string;
  driver: string;
  bus_id: number;
  type: 'wheel' | 'revolute' | 'gripper';
  ownership: 'platform' | 'module';
  invert?: boolean;
  command_interfaces: string[];
  state_interfaces: string[];
  limits?: {
    velocity_radps?: number;
    position_min_rad?: number;
    position_max_rad?: number;
  };
}

export interface PlatformDescriptor {
  schema: number;
  platform: { id: string; name?: string; vehicle_class: string };
  drivers: Record<string, { kind: string; port?: string; baud?: number }>;
  joints: Joint[];
  kinematics?: {
    model: string;
    wheel_radius_m: number;
    track_width_m: number;
    wheels: Record<string, string>;
  };
  modes?: { available: string[] };
  observations?: { streams: { subject: string; message: string; rate_hz: number }[] };
  actions?: { altitudes: string[] };
}

const KNOWN_VEHICLE_CLASSES = new Set(['diff_drive_rover', 'fixed_base']);

export function parseDescriptor(toml: string): PlatformDescriptor {
  const d = parse(toml) as unknown as PlatformDescriptor;
  if (d.schema !== 1) {
    throw new Error(`descriptor: unsupported schema ${d.schema}`);
  }
  if (!KNOWN_VEHICLE_CLASSES.has(d.platform.vehicle_class)) {
    throw new Error(`descriptor: unknown vehicle_class ${d.platform.vehicle_class}`);
  }
  if (d.platform.vehicle_class === 'diff_drive_rover' && !d.kinematics) {
    throw new Error('descriptor: diff_drive_rover requires [kinematics]');
  }
  if (d.platform.vehicle_class === 'fixed_base' && d.kinematics) {
    throw new Error('descriptor: fixed_base must not declare [kinematics]');
  }
  return d;
}

export function busIdForJoint(d: PlatformDescriptor, name: string): number | undefined {
  return d.joints.find((j) => j.name === name)?.bus_id;
}

/** Bus ids of platform-owned joints: the module-surface deny-list. */
export function platformOwnedBusIds(d: PlatformDescriptor): number[] {
  return d.joints.filter((j) => j.ownership === 'platform').map((j) => j.bus_id);
}

/**
 * Body twist to signed wheel angular velocities (rad/s) keyed by joint name,
 * inversion applied: the value a driver writes, not the rover-frame value.
 */
export function wheelSpeeds(
  d: PlatformDescriptor,
  vxMps: number,
  wzRadps: number,
): Record<string, number> {
  const k = d.kinematics;
  if (!k || k.model !== 'diff_drive') {
    throw new Error('descriptor: wheelSpeeds requires diff_drive kinematics');
  }
  const vL = vxMps - (wzRadps * k.track_width_m) / 2;
  const vR = vxMps + (wzRadps * k.track_width_m) / 2;
  const out: Record<string, number> = {};
  for (const [pos, name] of Object.entries(k.wheels)) {
    const v = pos === 'front_right' || pos === 'back_right' ? vR : vL;
    let w = v / k.wheel_radius_m;
    if (d.joints.find((j) => j.name === name)?.invert) w = -w;
    out[name] = w;
  }
  return out;
}
