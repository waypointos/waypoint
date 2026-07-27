import { describe, it, expect } from 'vitest';
import { resolveTabs, BUILTIN_TABS } from './tabs';
import { ModuleUI_Kind, ModuleOrigin } from '../../../../protocol/gen/ts/messages/modules_pb';
import type { ModuleInfo } from '../../../../protocol/gen/ts/messages/modules_pb';

function info(overrides: Partial<ModuleInfo> & { ui?: { kind: number; tabId: string; lanOnly: boolean } }): ModuleInfo {
  return {
    id: 'x', label: 'X', version: '0.1.0', tabId: '', healthy: true,
    origin: ModuleOrigin.UNSPECIFIED,
    lastSeen: undefined, ui: undefined, ...overrides,
  } as unknown as ModuleInfo;
}

describe('resolveTabs', () => {
  it('returns builtins when no modules', () => {
    expect(resolveTabs([], 'local').map(t => t.id)).toEqual(BUILTIN_TABS.map(t => t.id));
  });

  it('adds a static module tab when healthy + ui declared', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', label: 'Power Monitor', ui: { kind: ModuleUI_Kind.STATIC, tabId: 'm-pm', lanOnly: false } }),
    ], 'local');
    expect(tabs.find(t => t.id === 'm-pm')).toBeDefined();
  });

  it('hides lanOnly tabs when on proxy', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', ui: { kind: ModuleUI_Kind.PROXY, tabId: 'm-pm', lanOnly: true } }),
    ], 'proxy');
    expect(tabs.find(t => t.id === 'm-pm')).toBeUndefined();
  });

  it('shows lanOnly tabs when on local', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', ui: { kind: ModuleUI_Kind.PROXY, tabId: 'm-pm', lanOnly: true } }),
    ], 'local');
    expect(tabs.find(t => t.id === 'm-pm')).toBeDefined();
  });

  it('skips unhealthy modules', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', healthy: false, ui: { kind: ModuleUI_Kind.STATIC, tabId: 'm-pm', lanOnly: false } }),
    ], 'local');
    expect(tabs.find(t => t.id === 'm-pm')).toBeUndefined();
  });

  it('local mode: shows static module regardless of origin or lanOnly', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', origin: ModuleOrigin.LOCAL, ui: { kind: ModuleUI_Kind.STATIC, tabId: 'm-pm', lanOnly: true } }),
    ], 'local');
    expect(tabs.find(t => t.id === 'm-pm')).toBeDefined();
  });

  it('proxy mode: shows proxy-origin STATIC module', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', origin: ModuleOrigin.PROXY, ui: { kind: ModuleUI_Kind.STATIC, tabId: 'm-pm', lanOnly: false } }),
    ], 'proxy');
    expect(tabs.find(t => t.id === 'm-pm')).toBeDefined();
  });

  it('proxy mode: hides local-origin STATIC module', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', origin: ModuleOrigin.LOCAL, ui: { kind: ModuleUI_Kind.STATIC, tabId: 'm-pm', lanOnly: false } }),
    ], 'proxy');
    expect(tabs.find(t => t.id === 'm-pm')).toBeUndefined();
  });

  it('proxy mode: hides lan_only module even if proxy-origin STATIC', () => {
    const tabs = resolveTabs([
      info({ id: 'pm', origin: ModuleOrigin.PROXY, ui: { kind: ModuleUI_Kind.STATIC, tabId: 'm-pm', lanOnly: true } }),
    ], 'proxy');
    expect(tabs.find(t => t.id === 'm-pm')).toBeUndefined();
  });
});
