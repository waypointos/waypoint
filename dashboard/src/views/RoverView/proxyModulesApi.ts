// Typed client for the proxy's admin module-registry endpoints. Cookie-session
// (admin) auth; used only in proxy mode (see modulesApi.ts for the local agent API).

// Mirrors waypoint.v1.ModuleConfigField. The registry echoes a version's schema
// from its release manifest so the enable form can render fields before the
// module is attached and publishing its own schema on infra.modules.
export type ConfigFieldSpec = {
  key: string;
  label: string;
  type: string;
  defaultValue: string;
  help: string;
  required: boolean;
};

export type RegisteredVersion = {
  version: string;
  ingestedAt: string;
  configFields: ConfigFieldSpec[];
};
export type RegisteredModule = {
  moduleId: string;
  displayName: string;
  sourceRepoUrl: string;
  versions: RegisteredVersion[];
};
export type DesiredModule = { moduleId: string; version: string; configToml: string; updatedAt: string };

// Keeps thrown messages human-readable: an intermediary (Cloudflare, Railway)
// can answer with a whole HTML error page, which must never reach the UI.
async function errorText(r: Response): Promise<string> {
  const text = (await r.text()).trim();
  if (!text || text.startsWith('<')) return `request failed (HTTP ${r.status})`;
  const line = text.split('\n', 1)[0];
  return `${r.status}: ${line.length > 200 ? line.slice(0, 200) + '…' : line}`;
}

async function ok(r: Response): Promise<Response> {
  if (!r.ok) throw new Error(await errorText(r));
  return r;
}

export async function listRegistered(): Promise<RegisteredModule[]> {
  const r = await ok(await fetch('/api/admin/modules', { credentials: 'include' }));
  const body = (await r.json()) as {
    modules: Array<{
      module_id: string; display_name: string; source_repo_url: string;
      versions: Array<{
        version: string; ingested_at: string;
        config_fields?: Array<{ key: string; label: string; type: string; default: string; help: string; required: boolean }>;
      }>;
    }>;
  };
  return (body.modules ?? []).map((m) => ({
    moduleId: m.module_id,
    displayName: m.display_name,
    sourceRepoUrl: m.source_repo_url,
    versions: (m.versions ?? []).map((v) => ({
      version: v.version,
      ingestedAt: v.ingested_at,
      configFields: (v.config_fields ?? []).map((f) => ({
        key: f.key, label: f.label, type: f.type,
        defaultValue: f.default, help: f.help, required: f.required,
      })),
    })),
  }));
}

// The module id is not sent: the proxy derives it from the release manifest.
export async function registerModule(sourceRepoUrl: string, repoVisibility = 'public', githubToken = ''): Promise<void> {
  await ok(await fetch('/api/admin/modules', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source_repo_url: sourceRepoUrl, repo_visibility: repoVisibility, github_token: githubToken }),
  }));
}

export async function listRoverDesired(roverId: string): Promise<DesiredModule[]> {
  const r = await ok(await fetch(`/api/admin/rovers/${encodeURIComponent(roverId)}/modules`, { credentials: 'include' }));
  const body = (await r.json()) as { desired: Array<{ module_id: string; version: string; config_toml: string; updated_at: string }> };
  return (body.desired ?? []).map((d) => ({ moduleId: d.module_id, version: d.version, configToml: d.config_toml, updatedAt: d.updated_at }));
}

export async function setRoverDesired(roverId: string, moduleId: string, version: string, configToml: string): Promise<void> {
  await ok(await fetch(`/api/admin/rovers/${encodeURIComponent(roverId)}/modules/${encodeURIComponent(moduleId)}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ version, config_toml: configToml }),
  }));
}

// Unpins the module from one rover; its reconciler then detaches and stops it.
export async function unpinRoverModule(roverId: string, moduleId: string): Promise<void> {
  await ok(await fetch(`/api/admin/rovers/${encodeURIComponent(roverId)}/modules/${encodeURIComponent(moduleId)}`, {
    method: 'DELETE',
    credentials: 'include',
  }));
}

// Removes the module from the registry. The proxy refuses (409) while any
// rover still has it pinned.
export async function deleteModule(moduleId: string): Promise<void> {
  await ok(await fetch(`/api/admin/modules/${encodeURIComponent(moduleId)}`, {
    method: 'DELETE',
    credentials: 'include',
  }));
}

export type MarketplaceRover = {
  roverId: string;
  desiredVersion: string;
  autoUpdate: boolean;
  updateAvailable: boolean;
};
export type MarketplaceModule = {
  moduleId: string;
  displayName: string;
  sourceRepoUrl: string;
  repoVisibility: string;
  latestVersion: string;
  hasReadme: boolean;
  rovers: MarketplaceRover[];
};
export type CheckResult = { ingested: string[]; latest: string; autoUpdatedRovers: string[] };
// configToml omitted keeps whatever config the rover already has; the proxy only
// replaces it when the key is present.
export type DeployRover = { roverId: string; version: string; autoUpdate: boolean; configToml?: string };

export async function listMarketplace(): Promise<MarketplaceModule[]> {
  const r = await ok(await fetch('/api/admin/marketplace', { credentials: 'include' }));
  const body = (await r.json()) as {
    modules: Array<{
      module_id: string; display_name: string; source_repo_url: string; repo_visibility: string;
      latest_version: string; has_readme: boolean;
      rovers: Array<{ rover_id: string; desired_version: string; auto_update: boolean; update_available: boolean }>;
    }>;
  };
  return (body.modules ?? []).map((m) => ({
    moduleId: m.module_id, displayName: m.display_name, sourceRepoUrl: m.source_repo_url,
    repoVisibility: m.repo_visibility, latestVersion: m.latest_version, hasReadme: m.has_readme,
    rovers: (m.rovers ?? []).map((rv) => ({
      roverId: rv.rover_id, desiredVersion: rv.desired_version, autoUpdate: rv.auto_update, updateAvailable: rv.update_available,
    })),
  }));
}

export async function getModuleReadme(moduleId: string): Promise<string> {
  const r = await ok(await fetch(`/api/admin/modules/${encodeURIComponent(moduleId)}/readme`, { credentials: 'include' }));
  return ((await r.json()) as { markdown: string }).markdown ?? '';
}

export async function checkModuleUpdates(moduleId: string): Promise<CheckResult> {
  const r = await ok(await fetch(`/api/admin/modules/${encodeURIComponent(moduleId)}/check-updates`, { method: 'POST', credentials: 'include' }));
  const b = (await r.json()) as { ingested: string[]; latest: string; auto_updated_rovers: string[] };
  return { ingested: b.ingested ?? [], latest: b.latest ?? '', autoUpdatedRovers: b.auto_updated_rovers ?? [] };
}

export async function deployModule(moduleId: string, rovers: DeployRover[]): Promise<void> {
  await ok(await fetch(`/api/admin/modules/${encodeURIComponent(moduleId)}/deploy`, {
    method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      rovers: rovers.map((rv) => ({
        rover_id: rv.roverId,
        version: rv.version,
        auto_update: rv.autoUpdate,
        ...(rv.configToml === undefined ? {} : { config_toml: rv.configToml }),
      })),
    }),
  }));
}
