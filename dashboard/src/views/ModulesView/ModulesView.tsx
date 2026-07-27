import { useCallback, useEffect, useState } from 'react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Button } from '@/ui/primitives/Button';
import { ModuleCard } from './ModuleCard';
import { ModuleDetailDrawer } from './ModuleDetailDrawer';
import { RegisterModuleDialog } from './RegisterModuleDialog';
import { listMarketplace, checkModuleUpdates, type MarketplaceModule } from '@/views/RoverView/proxyModulesApi';
import styles from './ModulesView.module.css';

export function ModulesView() {
  const [modules, setModules] = useState<MarketplaceModule[]>([]);
  const [openId, setOpenId] = useState<string | null>(null);
  const [registering, setRegistering] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const refresh = useCallback(async () => {
    try {
      setModules(await listMarketplace());
    } catch (e) {
      setErr(String((e as Error).message ?? e));
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function checkAll() {
    setBusy(true);
    setErr('');
    try {
      await Promise.all(modules.map((m) => checkModuleUpdates(m.moduleId)));
      await refresh();
    } catch (e) {
      setErr(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  const open = modules.find((m) => m.moduleId === openId) ?? null;
  const updates = modules.reduce((n, m) => n + m.rovers.filter((r) => r.updateAvailable).length, 0);

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// MODULES</h1>
            <div className={styles.sub}>{modules.length} registered · {updates} update{updates === 1 ? '' : 's'} available</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <div className={styles.toolbar}>
            <Button onClick={checkAll} disabled={busy}>{busy ? 'Checking…' : 'Check all for updates'}</Button>
            <Button variant="primary" onClick={() => setRegistering(true)}>+ Register module</Button>
          </div>
          {err && <div className={styles.err}>{err}</div>}
          <div className={styles.grid}>
            {modules.map((m) => <ModuleCard key={m.moduleId} module={m} onOpen={setOpenId} />)}
          </div>
        </section>
      </main>

      {open && (
        <ModuleDetailDrawer module={open} open onClose={() => setOpenId(null)} onChanged={refresh} />
      )}
      <RegisterModuleDialog open={registering} onClose={() => setRegistering(false)} onDone={refresh} />
    </div>
  );
}
