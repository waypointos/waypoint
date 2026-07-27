import { ModuleInfo, ModuleOrigin } from '../../../../protocol/gen/ts/messages/modules_pb';
import type { Mode } from '../../state/mode';

export type TeleopWindowDef = {
  moduleId: string;
  windowId: string;
  label: string;
  entry: string;
  bindings: string[];
};

// Mirrors resolveTabs' visibility rule: local shows all; over the proxy only
// PROXY-origin modules surface (a locally-installed module's window must be
// registered through the proxy to appear there).
export function resolveTeleopWindows(modules: ModuleInfo[], sessionMode: Mode): TeleopWindowDef[] {
  return modules
    .filter((m) => m.healthy && m.ui?.teleop && m.ui.teleop.windowId.startsWith('w-'))
    .filter((m) => (sessionMode === 'local' ? true : m.origin === ModuleOrigin.PROXY))
    .map((m) => ({
      moduleId: m.id,
      windowId: m.ui!.teleop!.windowId,
      label: m.ui!.teleop!.label || m.id,
      entry: m.ui!.teleop!.entry,
      bindings: m.ui!.teleop!.bindings,
    }))
    // Snapshot order is not guaranteed stable; sort so the rail never reorders.
    .sort((a, b) => a.label.localeCompare(b.label) || a.windowId.localeCompare(b.windowId));
}
