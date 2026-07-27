// dashboard/src/views/AdminModulesView.tsx
//
// /admin/modules — admin-only no-rebuild module registry. Two Panels:
// REGISTER (paste a repo URL + id, POST /api/admin/modules) and
// REGISTERED MODULES (GET /api/admin/modules listing).
import { useEffect, useState } from 'react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Button } from '@/ui/primitives/Button';
import { Panel } from '@/ui/primitives/Panel';
import { registerModule } from '@/views/RoverView/proxyModulesApi';
import styles from './AdminModulesView.module.css';

type RegisteredModule = {
  module_id: string;
  display_name: string;
  source_repo_url: string;
  versions: { version: string; ingested_at: string }[];
};

export function AdminModulesView() {
  const [repoURL, setRepoURL] = useState('');
  const [modules, setModules] = useState<RegisteredModule[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    try {
      const resp = await fetch('/api/admin/modules');
      const json = await resp.json();
      setModules(json.modules ?? []);
    } catch {
      // Listing errors are secondary; registration UX is the priority.
      setModules([]);
    }
  }

  useEffect(() => { refresh(); }, []);

  async function register() {
    setBusy(true);
    setError(null);
    try {
      await registerModule(repoURL);
      setRepoURL('');
      await refresh();
    } catch (e) {
      setError(String((e as Error)?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// MODULES</h1>
            <div className={styles.sub}>No-rebuild module registry</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <Panel title="REGISTER">
            <div className={styles.form}>
              <label className={styles.field}>
                <span className={styles.label}>REPO URL</span>
                <input
                  className={styles.input}
                  type="url"
                  value={repoURL}
                  onChange={e => setRepoURL(e.target.value)}
                  placeholder="https://github.com/<org>/waypoint-module-<id>"
                />
              </label>
              <div className={styles.actions}>
                <Button
                  variant="primary"
                  onClick={register}
                  disabled={busy || !repoURL}
                >
                  {busy ? 'Registering…' : 'Register'}
                </Button>
                {error !== null && <span className={styles.error}>{error}</span>}
              </div>
            </div>
          </Panel>

          <Panel title="REGISTERED MODULES" note={`${modules.length} registered`}>
            {modules.length === 0 ? (
              <p className={styles.empty}>No modules registered yet.</p>
            ) : (
              <ul className={styles.list}>
                {modules.map(m => (
                  <li key={m.module_id} className={styles.row}>
                    <div className={styles.rowMain}>
                      <span className={styles.displayName}>{m.display_name}</span>
                      <span className={styles.moduleId}>{m.module_id}</span>
                    </div>
                    <div className={styles.rowMeta}>
                      <a href={m.source_repo_url} target="_blank" rel="noreferrer">
                        {m.source_repo_url}
                      </a>
                      <span className={styles.versionCount}>
                        {m.versions.length} version{m.versions.length === 1 ? '' : 's'}
                      </span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </section>
      </main>
    </div>
  );
}
