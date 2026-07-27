// dashboard/src/state/session.ts
//
// Bootstrap-token override. On the trusted LAN the agent auto-issues the
// wp_local_token cookie, so this is normally unused; it remains an override
// for off-LAN access (?token=...) and persists the value in localStorage.
const TOKEN_KEY = 'waypoint:token';

export function getToken(): string {
  // URL param wins (?token=...), then localStorage. Empty when unset so the
  // host returns 401 (a real auth error) instead of us sending a bogus token.
  const url = new URL(window.location.href);
  const fromUrl = url.searchParams.get('token');
  if (fromUrl) {
    localStorage.setItem(TOKEN_KEY, fromUrl);
    url.searchParams.delete('token');
    history.replaceState({}, '', url.toString());
    return fromUrl;
  }
  return localStorage.getItem(TOKEN_KEY) ?? '';
}
