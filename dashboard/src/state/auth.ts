// dashboard/src/state/auth.ts
//
// Proxy-mode login guard. In local mode the agent serves the dashboard and
// there is no login page — the bootstrap token handles auth.
//
// In proxy mode, the server-side gate (proxy/internal/router) is the primary
// barrier: anonymous users never reach this code. This function is a
// defensive backstop for stale-session edge cases (cookie present but DB row
// gone), where the server still returned the SPA but /api/me now 401s.
import { fetchMe } from './mode';

export async function requireLogin(): Promise<boolean> {
  const me = await fetchMe();
  if (me.mode === 'proxy' && !me.id) {
    const next = window.location.pathname + window.location.search;
    window.location.href = `/auth/login?next=${encodeURIComponent(next)}`;
    return false;
  }
  return true;
}
