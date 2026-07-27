// Typed client for the proxy's admin releases endpoints. Cookie-session
// (admin) auth; used only in proxy mode.

async function ok(r: Response): Promise<Response> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
  return r;
}

export type ReleaseRover = {
  roverId: string;
  name: string;
  currentVersion: string;
  channel: string;
  latestVersion: string;
  updateAvailable: boolean;
  swuUrl: string;
  swuSha256: string;
  releaseNotesMd: string;
  releaseHtmlUrl: string;
};

export async function listReleasesFleet(): Promise<ReleaseRover[]> {
  const r = await ok(await fetch('/api/admin/releases', { credentials: 'include' }));
  const body = (await r.json()) as {
    rovers: Array<{
      rover_id: string; name: string; current_version: string; channel: string;
      latest_version: string; update_available: boolean; swu_url: string; swu_sha256: string;
      release_notes_md: string; release_html_url: string;
    }>;
  };
  return (body.rovers ?? []).map((d) => ({
    roverId: d.rover_id, name: d.name, currentVersion: d.current_version, channel: d.channel,
    latestVersion: d.latest_version, updateAvailable: d.update_available, swuUrl: d.swu_url,
    swuSha256: d.swu_sha256, releaseNotesMd: d.release_notes_md, releaseHtmlUrl: d.release_html_url,
  }));
}

export async function checkImageUpdates(): Promise<number> {
  const r = await ok(await fetch('/api/admin/image-sources/check-updates', { method: 'POST', credentials: 'include' }));
  return ((await r.json()) as { ingested: number }).ingested ?? 0;
}

export async function registerImageSource(repoUrl: string, channel: string, repoVisibility = 'public', githubToken = ''): Promise<void> {
  await ok(await fetch('/api/admin/image-sources', {
    method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repo_url: repoUrl, channel, repo_visibility: repoVisibility, github_token: githubToken }),
  }));
}

export async function applyImage(roverId: string, url: string, expectedSha256: string, version: string): Promise<void> {
  const r = await fetch(`/api/rovers/${encodeURIComponent(roverId)}/apply-image`, {
    method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, expectedSha256, version }),
  });
  if (r.status !== 202) throw new Error((await r.text()) || `HTTP ${r.status}`);
}
