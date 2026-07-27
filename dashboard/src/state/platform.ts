// dashboard/src/state/platform.ts
//
// Runtime platform shape for the connected rover, fed by infra.platform
// (core's projection of the descriptor it booted with). Until the first
// message arrives, the baked rover descriptor is the fallback so existing
// local flows render identically.
import { useMemo } from 'react';
import { PlatformInfo } from '../../../protocol/gen/ts/messages/platform_pb';
import {
  parseDescriptor,
  type PlatformDescriptor,
} from '../../../protocol/platform/ts/descriptor';
import bakedToml from '../../../protocol/platform/waypoint-rover.toml?raw';
import { pbFromBinary } from './protobuf';
import { useTelemetry } from './useTelemetry';

export type PlatformJointShape = {
  name: string;
  busId: number;
  type: string;
  ownership: string;
  invert: boolean;
};

export type PlatformShape = {
  platformId: string;
  vehicleClass: string;
  hasDrive: boolean;
  kinematics?: {
    wheelRadiusM: number;
    trackWidthM: number;
    wheels: Record<string, string>; // position -> joint name
  };
  joints: PlatformJointShape[];
};

export function fromInfo(m: PlatformInfo): PlatformShape {
  return {
    platformId: m.platformId,
    vehicleClass: m.vehicleClass,
    hasDrive: m.kinematics !== undefined,
    kinematics: m.kinematics
      ? {
          wheelRadiusM: m.kinematics.wheelRadiusM,
          trackWidthM: m.kinematics.trackWidthM,
          wheels: { ...m.kinematics.wheels },
        }
      : undefined,
    joints: m.joints.map((j) => ({
      name: j.name,
      busId: j.busId,
      type: j.type,
      ownership: j.ownership,
      invert: j.invert,
    })),
  };
}

function fromDescriptor(d: PlatformDescriptor): PlatformShape {
  return {
    platformId: d.platform.id,
    vehicleClass: d.platform.vehicle_class,
    hasDrive: d.kinematics !== undefined,
    kinematics: d.kinematics
      ? {
          wheelRadiusM: d.kinematics.wheel_radius_m,
          trackWidthM: d.kinematics.track_width_m,
          wheels: { ...d.kinematics.wheels },
        }
      : undefined,
    joints: d.joints.map((j) => ({
      name: j.name,
      busId: j.bus_id,
      type: j.type,
      ownership: j.ownership,
      invert: j.invert ?? false,
    })),
  };
}

export const FALLBACK_PLATFORM: PlatformShape = fromDescriptor(
  parseDescriptor(bakedToml),
);

function decodePlatformInfo(b: Uint8Array): PlatformInfo {
  return pbFromBinary<PlatformInfo>(PlatformInfo, b);
}

/** Latest platform for the rover; baked rover descriptor until first message. */
export function usePlatform(roverId: string): PlatformShape {
  const info = useTelemetry(
    `waypoint.${roverId}.infra.platform`,
    decodePlatformInfo,
  );
  return useMemo(() => (info ? fromInfo(info) : FALLBACK_PLATFORM), [info]);
}

function wheelIdsByPositions(p: PlatformShape, positions: string[]): number[] {
  if (!p.kinematics) return [];
  const byName = new Map(p.joints.map((j) => [j.name, j.busId]));
  return positions
    .map((pos) => byName.get(p.kinematics!.wheels[pos]))
    .filter((id): id is number => id !== undefined);
}

export function leftWheelBusIds(p: PlatformShape): number[] {
  return wheelIdsByPositions(p, ['front_left', 'back_left']);
}

export function rightWheelBusIds(p: PlatformShape): number[] {
  return wheelIdsByPositions(p, ['front_right', 'back_right']);
}

export function wheelBusIds(p: PlatformShape): number[] {
  return [...leftWheelBusIds(p), ...rightWheelBusIds(p)];
}
