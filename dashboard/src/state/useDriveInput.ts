// dashboard/src/state/useDriveInput.ts
//
// Resolves the active drive input source. The on-screen JoystickPad feeds
// setVirtual; a connected gamepad takes over automatically (and the UI can
// switch back). Returns the active stick in normalized -1..1.
import { useCallback, useEffect, useState } from 'react';
import { useGamepadStick } from '@/lib/gamepad';

export type InputSource = 'virtual' | 'gamepad';
export type Stick = { vx: number; yaw: number };
const ZERO: Stick = { vx: 0, yaw: 0 };

export function useDriveInput() {
  const [source, setSource] = useState<InputSource>('virtual');
  const [gamepadConnected, setGamepadConnected] = useState(false);
  const [virtual, setVirtual] = useState<Stick>(ZERO);

  useEffect(() => {
    const anyPad = () => Array.from(navigator.getGamepads?.() ?? []).some((p) => p != null);
    const onConnect = () => { setGamepadConnected(true); setSource('gamepad'); };
    const onDisconnect = () => {
      const still = anyPad();
      setGamepadConnected(still);
      if (!still) setSource('virtual');
    };
    setGamepadConnected(anyPad());
    window.addEventListener('gamepadconnected', onConnect);
    window.addEventListener('gamepaddisconnected', onDisconnect);
    return () => {
      window.removeEventListener('gamepadconnected', onConnect);
      window.removeEventListener('gamepaddisconnected', onDisconnect);
    };
  }, []);

  const gamepad = useGamepadStick(source === 'gamepad');
  const stick: Stick = source === 'gamepad' ? (gamepad ?? ZERO) : virtual;

  const select = useCallback((s: InputSource) => setSource(s), []);
  return { source, setSource: select, gamepadConnected, setVirtual, stick };
}
