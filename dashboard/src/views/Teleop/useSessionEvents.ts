// dashboard/src/views/Teleop/useSessionEvents.ts
//
// Rolling buffer of operator-relevant happenings during a teleop session.
// Callers log entries; the EventLog component renders them.
import { useCallback, useState } from 'react';
import type { SessionEvent } from '@/ui/telemetry/EventLog';

const MAX_EVENTS = 24;

export function useSessionEvents() {
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const log = useCallback((text: string, tone?: SessionEvent['tone']) => {
    setEvents((prev) => [{ atMs: Date.now(), text, tone }, ...prev].slice(0, MAX_EVENTS));
  }, []);
  return { events, log };
}
