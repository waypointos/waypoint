// dashboard/src/lib/gamepad.ts
//
// Subscribes to the first connected Gamepad. Reads the left stick on each
// animation frame. Returns (vx, yaw) in -1..1 with a small deadzone.
import { useEffect, useRef, useState } from 'react';

const DEAD = 0.06;

export function useGamepadStick(active: boolean) {
  const [vec, setVec] = useState<{ vx: number; yaw: number } | null>(null);
  const raf = useRef(0);
  useEffect(() => {
    if (!active) return;
    const tick = () => {
      const pads = navigator.getGamepads?.() ?? [];
      const pad = Array.from(pads).find((p): p is Gamepad => p !== null);
      if (pad) {
        let x = pad.axes[0] ?? 0;
        let y = pad.axes[1] ?? 0;
        if (Math.abs(x) < DEAD) x = 0;
        if (Math.abs(y) < DEAD) y = 0;
        setVec({ vx: -y, yaw: x });
      }
      raf.current = requestAnimationFrame(tick);
    };
    raf.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf.current);
  }, [active]);
  return vec;
}
