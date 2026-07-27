// dashboard/src/views/RoverView/modulesApi.ts
//
// Typed client for the agent's local module management HTTP API.
// All requests forward the bootstrap token and include the session cookie.
import { getToken } from '@/state/session';

export function apiUrl(path: string): string {
  const token = getToken();
  const sep = path.includes('?') ? '&' : '?';
  return token ? `${path}${sep}token=${encodeURIComponent(token)}` : path;
}

export async function check(r: Response): Promise<Response> {
  if (!r.ok) throw new Error(await r.text());
  return r;
}

export type ModuleListItem = {
  id: string;
  label: string;
  version: string;
  origin: 'local' | 'proxy';
};

export type StageResult = {
  stageId: string;
  id: string;
  version: string;
  label: string;
  signerSan: string;
  uiKind: string;
  tabId: string;
  permissions: string[];
  devices: string[];
  pinned: boolean;
};

export async function listModules(): Promise<ModuleListItem[]> {
  const r = await check(await fetch(apiUrl('/api/modules'), { credentials: 'include' }));
  return (await r.json()) as ModuleListItem[];
}

export async function stageModule(raw: File, bundle: File): Promise<StageResult> {
  const form = new FormData();
  form.append('raw', raw);
  form.append('bundle', bundle);
  const r = await check(
    await fetch(apiUrl('/api/modules/stage'), {
      method: 'POST',
      credentials: 'include',
      body: form,
    }),
  );
  return (await r.json()) as StageResult;
}

export async function confirmModule(stageId: string, configToml: string): Promise<void> {
  await check(
    await fetch(apiUrl('/api/modules/confirm'), {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ stageId, configToml }),
    }),
  );
}

export async function uninstallModule(id: string): Promise<void> {
  await check(
    await fetch(apiUrl(`/api/modules/${encodeURIComponent(id)}`), {
      method: 'DELETE',
      credentials: 'include',
    }),
  );
}
