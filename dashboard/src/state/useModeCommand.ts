// dashboard/src/state/useModeCommand.ts
//
// Optimistic mode/estop/recover control. We publish the command immediately and
// hold a "pending" intent until core's event.mode (passed in as currentMode)
// confirms the new state, or a timeout elapses (rejected / no response). The
// rover is authoritative; this only drives transient UI feedback.
import { useCallback, useEffect, useRef, useState } from 'react';
import type { Mode as DisplayMode } from '@/ui/telemetry/ModeIndicator';
import { setMode, estop, recover, type ModeTarget } from './roverCommands';

const TIMEOUT_MS = 1500;

export type PendingIntent = ModeTarget | 'estop' | 'clearing' | null;

export function useModeCommand(roverId: string, currentMode: DisplayMode) {
  const [pending, setPending] = useState<PendingIntent>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearTimer = () => {
    if (timer.current) { clearTimeout(timer.current); timer.current = null; }
  };
  const arm = useCallback((intent: Exclude<PendingIntent, null>) => {
    clearTimer();
    setPending(intent);
    timer.current = setTimeout(() => setPending(null), TIMEOUT_MS);
  }, []);

  useEffect(() => {
    if (!pending) return;
    const reached =
      (pending === 'manual'     && currentMode === 'manual')     ||
      (pending === 'safe'       && currentMode === 'safe')       ||
      (pending === 'autonomous' && currentMode === 'autonomous') ||
      (pending === 'estop'      && currentMode === 'estop')      ||
      (pending === 'clearing'   && currentMode !== 'estop' && currentMode !== 'unknown');
    if (reached) { clearTimer(); setPending(null); }
  }, [currentMode, pending]);

  useEffect(() => () => clearTimer(), []);

  const requestMode = useCallback((t: ModeTarget) => { arm(t); setMode(roverId, t); }, [arm, roverId]);
  const requestEstop = useCallback(() => { arm('estop'); estop(roverId); }, [arm, roverId]);
  const requestRecover = useCallback(() => { arm('clearing'); recover(roverId); }, [arm, roverId]);

  return { pending, requestMode, requestEstop, requestRecover };
}
