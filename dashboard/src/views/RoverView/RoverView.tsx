import { useParams, Navigate } from 'react-router-dom';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { Crumbs } from '@/ui/nav/Crumbs';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { AlertCountBadge } from '@/ui/nav/AlertCountBadge';
import { Chip } from '@/ui/primitives/Chip';
import { ConnectionPill } from '@/ui/telemetry/ConnectionPill';
import { ModeIndicator } from '@/ui/telemetry/ModeIndicator';
import { TabStrip } from '@/ui/nav/TabStrip';
import { useMe } from '@/state/mode';

import { RoverContextProvider, type Role } from './RoverContext';
import { useRoverData } from './useRoverData';
import { resolveTabs, DEFAULT_TAB_ID } from './tabs';
import styles from './RoverView.module.css';

export function RoverView() {
  const { id = 'rover-dev', tab } = useParams<{ id: string; tab?: string }>();
  const me = useMe();
  const ctx = useRoverData(id);
  const { role, connection, mode, modules, sys, alertCounts } = ctx;

  const tabs = resolveTabs(modules, me?.mode ?? 'offline');
  const activeTab = tab ? tabs.find((t) => t.path === tab) : undefined;
  if (!tab || !activeTab) {
    return <Navigate to={`/rover/${id}/${DEFAULT_TAB_ID}`} replace />;
  }

  const visibleTabs = tabs.filter((t) => roleAllows(role, t.minRole));

  return (
    <div className={styles.shell}>
      <SidebarBrand collapsed />
      <TopBar
        left={<Crumbs items={[{ label: 'FLEET', to: '/' }, { label: id.toUpperCase() }]} />}
        right={
          <>
            <OperatorClock />
            <ModeIndicator mode={mode} />
            <ConnectionPill conn={connection} />
            {sys?.imageVersion && <Chip>image {sys.imageVersion}</Chip>}
            <AlertCountBadge counts={alertCounts} />
          </>
        }
      />
      <Sidebar collapsed />
      <main className={styles.body}>
        <TabStrip
          tabs={visibleTabs.map((t) => ({ id: t.id, label: t.label, to: `/rover/${id}/${t.path}` }))}
        />
        <RoverContextProvider value={ctx}>
          <div className={styles.panels}>
            {visibleTabs.map((t) => {
              const Panel = t.panel;
              const active = t.id === activeTab.id;
              return (
                <div
                  key={t.id}
                  role="tabpanel"
                  data-active={active}
                  style={{ display: active ? 'block' : 'none' }}
                  className={styles.panel}
                >
                  <Panel roverId={id} active={active} />
                </div>
              );
            })}
          </div>
        </RoverContextProvider>
      </main>
    </div>
  );
}

function roleAllows(role: Role, min?: Role): boolean {
  if (!min) return true;
  const rank: Record<Role, number> = { monitor: 0, control: 1, admin: 2 };
  return rank[role] >= rank[min];
}
