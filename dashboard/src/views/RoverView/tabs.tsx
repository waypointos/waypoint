import type { ComponentType } from 'react';
import type { Role } from './RoverContext';
import { ModuleInfo, ModuleUI_Kind, ModuleOrigin } from '../../../../protocol/gen/ts/messages/modules_pb';
import type { Mode } from '../../state/mode';
import { OverviewPanel } from './panels/OverviewPanel';
import { ControlPanel } from './panels/ControlPanel';
import { SystemPanel } from './panels/SystemPanel';
import { LogsPanel } from './panels/LogsPanel';
import { ModulesManagePanel } from './panels/ModulesManagePanel';
import { EpisodesPanel } from './panels/EpisodesPanel';
import { ModulePanel } from './ModulePanel';
import { ModuleIframe } from './ModuleIframe';
import { ModuleTabFrame } from './ModuleTabFrame';

export type RoverTab = {
  id: string;
  label: string;
  path: string;             // segment after /rover/:id/
  panel: ComponentType<{ roverId?: string; active?: boolean }>;
  minRole?: Role;           // undefined = monitor can see it
  moduleId?: string;
};

export const BUILTIN_TABS: RoverTab[] = [
  { id: 'overview', label: 'OVERVIEW', path: 'overview', panel: OverviewPanel as ComponentType<{ roverId?: string; active?: boolean }> },
  { id: 'control',  label: 'CONTROL',  path: 'control',  panel: ControlPanel  as ComponentType<{ roverId?: string; active?: boolean }> },
  { id: 'system',   label: 'SYSTEM',   path: 'system',   panel: SystemPanel   as ComponentType<{ roverId?: string; active?: boolean }> },
  { id: 'logs',     label: 'LOGS',     path: 'logs',     panel: LogsPanel     as ComponentType<{ roverId?: string; active?: boolean }> },
  { id: 'modules',  label: 'MODULES',  path: 'modules',  panel: ModulesManagePanel, minRole: 'admin' },
  { id: 'episodes', label: 'EPISODES', path: 'episodes', panel: EpisodesPanel as ComponentType<{ roverId?: string; active?: boolean }> },
];

export const DEFAULT_TAB_ID = 'overview';

// Stable panel-component identity per (kind, moduleId). resolveTabs runs on every
// snapshot tick; without this cache each call would mint a new component function,
// remounting the module panel every tick (constant reload, repeated mount()).
const modulePanelCache = new Map<string, ComponentType<{ roverId?: string; active?: boolean }>>();
function modulePanelFor(moduleId: string, kind: ModuleUI_Kind): ComponentType<{ roverId?: string; active?: boolean }> {
  const key = `${kind}:${moduleId}`;
  let comp = modulePanelCache.get(key);
  if (!comp) {
    comp = ({ roverId }) => (
      <ModuleTabFrame moduleId={moduleId} roverId={roverId ?? ''}>
        {kind === ModuleUI_Kind.PROXY ? (
          <ModuleIframe moduleId={moduleId} />
        ) : (
          <ModulePanel moduleId={moduleId} roverId={roverId ?? ''} />
        )}
      </ModuleTabFrame>
    );
    modulePanelCache.set(key, comp);
  }
  return comp;
}

export function resolveTabs(modules: ModuleInfo[], sessionMode: Mode): RoverTab[] {
  const moduleTabs: RoverTab[] = modules
    .filter(m => m.healthy && m.ui && m.ui.tabId.startsWith('m-'))
    .filter(m => {
      if (sessionMode === 'local') return true;
      if (m.ui!.lanOnly) return false;
      // Off-local, only proxy-origin STATIC panels are servable (the proxy holds the .raw).
      return m.ui!.kind === ModuleUI_Kind.STATIC && m.origin === ModuleOrigin.PROXY;
    })
    .map(m => {
      const ui = m.ui!;
      const moduleId = m.id;
      const panel = modulePanelFor(moduleId, ui.kind);
      return {
        id:       ui.tabId,
        label:    (m.label || moduleId).toUpperCase(),
        path:     ui.tabId,
        panel,
        moduleId,
      };
    })
    // Snapshot order is not guaranteed stable; sort so tabs never swap places.
    .sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id));
  return [...BUILTIN_TABS, ...moduleTabs];
}
