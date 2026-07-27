// Loader for module UI bundles (panel.js / teleop.js). Tests stub the load by
// assigning globalThis.__waypointImport.
declare global {
  // eslint-disable-next-line no-var
  var __waypointImport: ((url: string) => Promise<unknown>) | undefined;
}

export function waypointImport<T>(url: string): Promise<T> {
  return (globalThis.__waypointImport ?? ((u: string) => import(/* @vite-ignore */ u)))(
    url,
  ) as Promise<T>;
}

// moduleStaticUrl resolves the URL for a file in a module's static tree.
// Local: the agent serves the mounted image's tree at its own origin. Proxy:
// the proxy serves the tree it extracted from the verified .raw — but a
// dynamic import() drops the session cookie from module-script requests, so
// the script URL carries a short-lived token minted over an authenticated
// fetch instead. The bundle's own relative asset fetches (models etc.)
// resolve against this URL and ride the session cookie like any admin call.
// Token minting is best-effort: on failure the plain URL still serves
// browsers that keep cookies on import().
export async function moduleStaticUrl(
  mode: string | undefined,
  roverId: string,
  moduleId: string,
  file: string,
): Promise<string> {
  if (mode === 'local') return `/module/${moduleId}/static/${file}`;
  const base = `/api/admin/rovers/${roverId}/modules/${moduleId}/static/${file}`;
  try {
    const r = await fetch(
      `/api/admin/rovers/${roverId}/modules/${moduleId}/static-token`,
      { credentials: 'include' },
    );
    if (!r.ok) return base;
    const { token } = (await r.json()) as { token: string };
    return `${base}?st=${encodeURIComponent(token)}`;
  } catch {
    return base;
  }
}
