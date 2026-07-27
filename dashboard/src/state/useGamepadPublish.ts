// dashboard/src/state/useGamepadPublish.ts
//
// Publishes one device-generic GamepadSnapshot to waypoint.<rover>.cmd.gamepad
// at ~50Hz whenever a physical gamepad is connected. Additive to drive: this
// does NOT replace useDriveInput/useDriveCmd. The agent mirrors cmd.gamepad
// into the subtree of any module that declares requires=["teleop-input"].
import { useEffect } from 'react';
import { Timestamp } from '@bufbuild/protobuf';
import { GamepadSnapshot } from '../../../protocol/gen/ts/messages/gamepad_pb';
import { Stamp } from '../../../protocol/gen/ts/messages/common_pb';
import { getBus } from './nats';
import { pbToBinary } from './protobuf';

const DEAD = 0.06;

function dz(v: number): number {
  return Math.abs(v) < DEAD ? 0 : v;
}

// WebKit (Safari) exposes "Extended Gamepad" controllers (e.g. DualSense) with
// mapping === "standard" but orders the four front buttons as [L2, R2, L1, R1]
// at indices 4..7, whereas the W3C standard order is [L1, R1, L2, R2]. Chromium
// already uses the standard order (its gamepad id carries "STANDARD GAMEPAD").
// Swap the two pairs back so every downstream consumer — the trigger extraction
// below and the arm module's MapInput — sees the canonical layout: bumpers at
// 4/5, analog triggers at 6/7.
function normalizedButtons(pad: Gamepad): readonly GamepadButton[] {
  if (!pad.id?.includes('Extended Gamepad') || pad.buttons.length < 8) return pad.buttons;
  const b = pad.buttons.slice();
  [b[4], b[6]] = [b[6], b[4]]; // L1 <-> L2
  [b[5], b[7]] = [b[7], b[5]]; // R1 <-> R2
  return b;
}

export function useGamepadPublish(roverId: string) {
  const subject = `waypoint.${roverId}.cmd.gamepad`;
  useEffect(() => {
    const id = setInterval(() => {
      const pads = navigator.getGamepads?.() ?? [];
      const pad = Array.from(pads).find((p) => p != null);
      if (!pad) return;
      const buttons = normalizedButtons(pad);
      const snap = new GamepadSnapshot({
        t: Timestamp.fromDate(new Date()),
        stamp: new Stamp({ t: Timestamp.fromDate(new Date()) }),
        axes: pad.axes.map(dz),
        buttons: buttons.map((b) => b.pressed),
        triggers: buttons.map((b) => b.value).filter((_, i) => i === 6 || i === 7),
      });
      getBus().publish(subject, pbToBinary(snap));
    }, 20); // 50 Hz
    return () => clearInterval(id);
  }, [subject]);
}
