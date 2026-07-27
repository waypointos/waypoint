import { describe, it, expect, vi, afterEach } from 'vitest';
import { moduleStaticUrl } from './moduleImport';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('moduleStaticUrl', () => {
  it('serves local files from the agent origin without a token', async () => {
    const url = await moduleStaticUrl('local', 'cosmos', 'so100', 'panel.js');
    expect(url).toBe('/module/so100/static/panel.js');
  });

  it('appends a minted token to proxy script URLs', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ token: '123.abc' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    const url = await moduleStaticUrl('proxy', 'cosmos', 'so100', 'panel.js');
    expect(url).toBe('/api/admin/rovers/cosmos/modules/so100/static/panel.js?st=123.abc');
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/admin/rovers/cosmos/modules/so100/static-token',
      { credentials: 'include' },
    );
  });

  it('falls back to the plain URL when minting fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }));
    expect(await moduleStaticUrl('proxy', 'cosmos', 'so100', 'teleop.js')).toBe(
      '/api/admin/rovers/cosmos/modules/so100/static/teleop.js',
    );
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    expect(await moduleStaticUrl(undefined, 'cosmos', 'so100', 'teleop.js')).toBe(
      '/api/admin/rovers/cosmos/modules/so100/static/teleop.js',
    );
  });
});
