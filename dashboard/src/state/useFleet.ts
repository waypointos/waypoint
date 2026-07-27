// dashboard/src/state/useFleet.ts
//
// Polls GET /api/rovers (proxy-side) every 5 s to get the list of rovers the
// current user has access to. The poll only runs in proxy mode — the local
// agent has no /api/rovers handler, so off-proxy the hook returns [] without
// touching the network.
import { useCallback, useEffect, useRef, useState } from 'react';
import { useMe } from './mode';

export interface FleetRover {
  id: string;
  name: string;
  role: 'monitor' | 'control' | 'admin';
  lastSeen?: string;
  imageVersion?: string;
}

export function useFleet(): {
  rovers: FleetRover[];
  loading: boolean;
  refresh: () => Promise<void>;
} {
  const me = useMe();
  const proxy = me?.mode === 'proxy';
  const [rovers, setRovers] = useState<FleetRover[]>([]);
  const [loading, setLoading] = useState(true);
  const cancelledRef = useRef(false);

  const tick = useCallback(async () => {
    try {
      const r = await fetch('/api/rovers', { credentials: 'include' });
      if (r.ok) {
        const data = await r.json();
        if (!cancelledRef.current) setRovers(data || []);
      }
    } finally {
      if (!cancelledRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    // /api/rovers is proxy-only; off-proxy there is no fleet to poll.
    if (!proxy) {
      setRovers([]);
      setLoading(false);
      return;
    }
    cancelledRef.current = false;
    tick();
    const h = setInterval(tick, 5000);
    return () => {
      cancelledRef.current = true;
      clearInterval(h);
    };
  }, [tick, proxy]);

  return { rovers, loading, refresh: tick };
}

/** Returns true if lastSeen ISO string is within the last 30 seconds. */
export function isOnline(lastSeen?: string): boolean {
  if (!lastSeen) return false;
  const ago = Date.now() - new Date(lastSeen).getTime();
  return ago < 30_000;
}
