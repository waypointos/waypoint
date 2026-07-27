import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { AdminModulesView } from './AdminModulesView';

function renderView() {
  return render(
    <MemoryRouter initialEntries={['/admin/modules']}>
      <AdminModulesView />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  globalThis.fetch = vi.fn(async (url, init) => {
    if (String(url).endsWith('/api/admin/modules') && (init?.method ?? 'GET') === 'POST') {
      return new Response('', { status: 200 });
    }
    return new Response(JSON.stringify({ modules: [] }), { status: 200 });
  }) as any;
});

describe('AdminModulesView', () => {
  it('POSTs the repo URL when the operator clicks Register', async () => {
    renderView();
    fireEvent.change(screen.getByLabelText(/repo URL/i), {
      target: { value: 'https://github.com/x/waypoint-module-power-monitor' },
    });
    fireEvent.click(screen.getByRole('button', { name: /register/i }));
    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/admin/modules',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('waypoint-module-power-monitor'),
        }),
      );
    });
  });

  it('lists registered modules from GET /api/admin/modules', async () => {
    globalThis.fetch = vi.fn(async () => {
      return new Response(JSON.stringify({
        modules: [{
          module_id: 'power-monitor',
          display_name: 'Power Monitor',
          source_repo_url: 'https://github.com/x/waypoint-module-power-monitor',
          versions: [{ version: '0.1.0', ingested_at: '2026-05-20T00:00:00Z' }],
        }],
      }), { status: 200 });
    }) as any;
    renderView();
    await waitFor(() => expect(screen.getByText(/Power Monitor/)).toBeInTheDocument());
    // The module_id "power-monitor" also appears inside the repo URL, so there
    // are multiple matches — verify at least one is rendered.
    expect(screen.getAllByText(/power-monitor/).length).toBeGreaterThan(0);
  });
});
