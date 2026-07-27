import { describe, it, expect, vi, beforeEach } from 'vitest';
import { listReleasesFleet, checkImageUpdates, registerImageSource, applyImage } from './releasesApi';

beforeEach(() => vi.restoreAllMocks());

describe('releasesApi', () => {
  it('listReleasesFleet maps snake_case', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      rovers: [{ rover_id: 'r1', name: 'R1', current_version: '0.5.0', channel: 'prod',
        latest_version: '0.6.0', update_available: true, swu_url: 'u', swu_sha256: 's',
        release_notes_md: '## n', release_html_url: 'h' }],
    }), { status: 200 }));
    const fleet = await listReleasesFleet();
    expect(fleet[0].roverId).toBe('r1');
    expect(fleet[0].latestVersion).toBe('0.6.0');
    expect(fleet[0].updateAvailable).toBe(true);
    expect(fleet[0].releaseNotesMd).toBe('## n');
  });

  it('applyImage posts url/sha/version', async () => {
    const spy = vi.spyOn(global, 'fetch').mockResolvedValue(new Response('', { status: 202 }));
    await applyImage('r1', 'https://dl/x.swu', 'abc', '0.6.0');
    const [url, init] = spy.mock.calls[0];
    expect(String(url)).toBe('/api/rovers/r1/apply-image');
    expect(JSON.parse(String(init?.body))).toEqual({ url: 'https://dl/x.swu', expectedSha256: 'abc', version: '0.6.0' });
  });

  it('checkImageUpdates posts', async () => {
    const spy = vi.spyOn(global, 'fetch').mockResolvedValue(new Response('{"ingested":2}', { status: 200 }));
    const n = await checkImageUpdates();
    expect(n).toBe(2);
    expect(String(spy.mock.calls[0][0])).toBe('/api/admin/image-sources/check-updates');
  });

  it('registerImageSource posts snake_case', async () => {
    const spy = vi.spyOn(global, 'fetch').mockResolvedValue({ ok: true, status: 204, text: async () => '' } as unknown as Response);
    await registerImageSource('https://github.com/acme/img', 'prod', 'public', '');
    expect(JSON.parse(String(spy.mock.calls[0][1]?.body))).toEqual({
      repo_url: 'https://github.com/acme/img', channel: 'prod', repo_visibility: 'public', github_token: '',
    });
  });
});
