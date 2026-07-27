import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { deleteModule, listMarketplace, deployModule, checkModuleUpdates, getModuleReadme, listRegistered, listRoverDesired, registerModule, setRoverDesired, unpinRoverModule } from './proxyModulesApi';

function makeFetch(status: number, body: unknown): ReturnType<typeof vi.fn> {
  return vi.fn(async () => ({
    ok: status >= 200 && status < 300,
    status,
    text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
    json: async () => body,
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('listRegistered', () => {
  it('maps snake_case fields to camelCase including nested versions', async () => {
    vi.stubGlobal('fetch', makeFetch(200, {
      modules: [
        {
          module_id: 'power-monitor',
          display_name: 'Power Monitor',
          source_repo_url: 'https://github.com/example/power-monitor',
          versions: [
            { version: '0.1.0', ingested_at: '2026-05-01T00:00:00Z' },
            { version: '0.2.0', ingested_at: '2026-05-10T00:00:00Z' },
          ],
        },
      ],
    }));

    const result = await listRegistered();

    expect(result).toHaveLength(1);
    expect(result[0].moduleId).toBe('power-monitor');
    expect(result[0].displayName).toBe('Power Monitor');
    expect(result[0].sourceRepoUrl).toBe('https://github.com/example/power-monitor');
    expect(result[0].versions).toHaveLength(2);
    expect(result[0].versions[0].version).toBe('0.1.0');
    expect(result[0].versions[0].ingestedAt).toBe('2026-05-01T00:00:00Z');
    expect(result[0].versions[1].ingestedAt).toBe('2026-05-10T00:00:00Z');
  });

  it('handles an empty modules array', async () => {
    vi.stubGlobal('fetch', makeFetch(200, { modules: [] }));
    expect(await listRegistered()).toEqual([]);
  });

  it('fetches GET /api/admin/modules with credentials:include', async () => {
    const spy = makeFetch(200, { modules: [] });
    vi.stubGlobal('fetch', spy);

    await listRegistered();

    expect(spy).toHaveBeenCalledWith('/api/admin/modules', { credentials: 'include' });
  });
});

describe('registerModule', () => {
  it('POSTs to /api/admin/modules with correct body and credentials', async () => {
    const spy = makeFetch(200, null);
    vi.stubGlobal('fetch', spy);

    await registerModule('https://github.com/example/power-monitor');

    expect(spy).toHaveBeenCalledWith('/api/admin/modules', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source_repo_url: 'https://github.com/example/power-monitor', repo_visibility: 'public', github_token: '' }),
    });
  });

  it('resolves without returning a value on success', async () => {
    vi.stubGlobal('fetch', makeFetch(200, null));
    await expect(registerModule('https://example.com/repo')).resolves.toBeUndefined();
  });
});

describe('listRoverDesired', () => {
  it('maps snake_case desired fields to camelCase', async () => {
    vi.stubGlobal('fetch', makeFetch(200, {
      desired: [
        {
          module_id: 'power-monitor',
          version: '0.2.0',
          config_toml: '[power]\npoll_ms = 1000\n',
          updated_at: '2026-05-20T12:00:00Z',
        },
      ],
    }));

    const result = await listRoverDesired('rover-1');

    expect(result).toHaveLength(1);
    expect(result[0].moduleId).toBe('power-monitor');
    expect(result[0].version).toBe('0.2.0');
    expect(result[0].configToml).toBe('[power]\npoll_ms = 1000\n');
    expect(result[0].updatedAt).toBe('2026-05-20T12:00:00Z');
  });

  it('fetches the correct rover-scoped URL with encodeURIComponent', async () => {
    const spy = makeFetch(200, { desired: [] });
    vi.stubGlobal('fetch', spy);

    await listRoverDesired('rover/with/slashes');

    expect(spy).toHaveBeenCalledWith(
      '/api/admin/rovers/rover%2Fwith%2Fslashes/modules',
      { credentials: 'include' },
    );
  });

  it('handles an empty desired array', async () => {
    vi.stubGlobal('fetch', makeFetch(200, { desired: [] }));
    expect(await listRoverDesired('rover-1')).toEqual([]);
  });
});

describe('setRoverDesired', () => {
  it('POSTs to the rover-module path with version and config_toml', async () => {
    const spy = makeFetch(200, null);
    vi.stubGlobal('fetch', spy);

    await setRoverDesired('rover-1', 'power-monitor', '0.2.0', '[power]\n');

    expect(spy).toHaveBeenCalledWith(
      '/api/admin/rovers/rover-1/modules/power-monitor',
      {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: '0.2.0', config_toml: '[power]\n' }),
      },
    );
  });

  it('encodes rover and module IDs in the URL', async () => {
    const spy = makeFetch(200, null);
    vi.stubGlobal('fetch', spy);

    await setRoverDesired('rover/1', 'mod/x', '1.0.0', '');

    const [url] = spy.mock.calls[0] as [string, unknown];
    expect(url).toBe('/api/admin/rovers/rover%2F1/modules/mod%2Fx');
  });
});

describe('ok — error path', () => {
  it('throws with status code and body text on a non-2xx response', async () => {
    vi.stubGlobal('fetch', makeFetch(500, 'internal server error'));

    await expect(listRegistered()).rejects.toThrow('500: internal server error');
  });

  it('throws on 404 with the response body', async () => {
    vi.stubGlobal('fetch', makeFetch(404, 'not found'));
    await expect(listRoverDesired('no-such-rover')).rejects.toThrow('404: not found');
  });
});

describe('proxyModulesApi marketplace', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('listMarketplace maps snake_case to camelCase', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      modules: [{
        module_id: 'power', display_name: 'Power', source_repo_url: 'u', repo_visibility: 'public',
        latest_version: '1.5.0', has_readme: true,
        rovers: [{ rover_id: 'r1', desired_version: '1.4.2', auto_update: true, update_available: true }],
      }],
    }), { status: 200 }));
    const mods = await listMarketplace();
    expect(mods[0].moduleId).toBe('power');
    expect(mods[0].latestVersion).toBe('1.5.0');
    expect(mods[0].rovers[0].updateAvailable).toBe(true);
  });

  it('deployModule posts rovers array', async () => {
    const spy = vi.spyOn(global, 'fetch').mockResolvedValue(new Response('{"results":[]}', { status: 200 }));
    await deployModule('power', [{ roverId: 'r1', version: '1.5.0', autoUpdate: true, configToml: '' }]);
    const [, init] = spy.mock.calls[0];
    expect(JSON.parse(String(init?.body))).toEqual({ rovers: [{ rover_id: 'r1', version: '1.5.0', auto_update: true, config_toml: '' }] });
  });

  it('checkModuleUpdates returns ingested', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue(new Response(JSON.stringify({ ingested: ['1.5.0'], latest: '1.5.0', auto_updated_rovers: [] }), { status: 200 }));
    const res = await checkModuleUpdates('power');
    expect(res.ingested).toEqual(['1.5.0']);
  });

  it('getModuleReadme returns markdown', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue(new Response(JSON.stringify({ markdown: '# x' }), { status: 200 }));
    expect(await getModuleReadme('power')).toBe('# x');
  });
});

describe('unpinRoverModule', () => {
  it('DELETEs the rover-module pairing', async () => {
    const spy = makeFetch(200, null);
    vi.stubGlobal('fetch', spy);
    await unpinRoverModule('rover-1', 'so100');
    expect(spy).toHaveBeenCalledWith('/api/admin/rovers/rover-1/modules/so100', {
      method: 'DELETE',
      credentials: 'include',
    });
  });
});

describe('deleteModule', () => {
  it('DELETEs the registry module', async () => {
    const spy = makeFetch(200, null);
    vi.stubGlobal('fetch', spy);
    await deleteModule('so100');
    expect(spy).toHaveBeenCalledWith('/api/admin/modules/so100', {
      method: 'DELETE',
      credentials: 'include',
    });
  });
});
