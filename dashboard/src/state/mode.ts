// dashboard/src/state/mode.ts
//
// Detects whether the dashboard is running against the local agent or the
// cloud proxy by calling GET /api/me. The result is cached for the session.
import { useState, useEffect } from 'react';

export type Mode = 'local' | 'proxy' | 'offline';

export interface Me {
  id?: string;
  email?: string;
  isAdmin?: boolean;
  mode: Mode;
  roverId?: string;
}

let cached: Me | null = null;
let inflight: Promise<Me> | null = null;

export async function fetchMe(): Promise<Me> {
  if (cached) return cached;
  if (inflight) return inflight;
  inflight = (async () => {
    try {
      const r = await fetch('/api/me', { credentials: 'include' });
      if (r.status === 401) return { mode: 'proxy' } as Me;
      if (!r.ok) return { mode: 'offline' } as Me;
      const me = (await r.json()) as Me;
      cached = me;
      return me;
    } catch {
      return { mode: 'offline' } as Me;
    } finally {
      inflight = null;
    }
  })();
  return inflight;
}

export function resetModeCache() {
  cached = null;
}

export function useMe(): Me | null {
  const [me, setMe] = useState<Me | null>(null);
  useEffect(() => {
    fetchMe().then(setMe);
  }, []);
  return me;
}
