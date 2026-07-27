import { describe, it, expect } from 'vitest';
import { ModuleInfo, ModuleUI, TeleopWindow, ModuleOrigin } from '../../../../protocol/gen/ts/messages/modules_pb';
import { resolveTeleopWindows } from './teleopWindows';

function mod(id: string, healthy: boolean, win?: Partial<TeleopWindow>, origin = ModuleOrigin.LOCAL): ModuleInfo {
  return new ModuleInfo({
    id, healthy, origin,
    ui: new ModuleUI({ teleop: win ? new TeleopWindow({ windowId: 'w-' + id, label: id, entry: 'teleop.js', ...win }) : undefined }),
  });
}

describe('resolveTeleopWindows', () => {
  it('lists healthy modules that advertise a teleop window', () => {
    const out = resolveTeleopWindows([mod('so100', true, {}), mod('x', true)], 'local');
    expect(out.map((w) => w.moduleId)).toEqual(['so100']);
    expect(out[0].entry).toBe('teleop.js');
  });

  it('hides unhealthy modules', () => {
    expect(resolveTeleopWindows([mod('so100', false, {})], 'local')).toEqual([]);
  });

  it('over proxy, only PROXY-origin windows show', () => {
    const local = mod('so100', true, {}, ModuleOrigin.LOCAL);
    const proxy = mod('p', true, {}, ModuleOrigin.PROXY);
    expect(resolveTeleopWindows([local, proxy], 'proxy').map((w) => w.moduleId)).toEqual(['p']);
  });
});
