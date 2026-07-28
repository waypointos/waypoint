// dashboard/src/views/RoverView/panels/ModulesManagePanel.tsx
//
// Module management panel — behavior splits by mode:
//   LOCAL:  upload .raw + cosign bundle, verify signer SAN, install/uninstall via agent HTTP API.
//   PROXY:  show running snapshot, registry list with per-rover desired state, register new modules.
import { useEffect, useRef, useState } from 'react';
import { Settings } from 'lucide-react';
import { Panel } from '@/ui/primitives/Panel';
import { Button } from '@/ui/primitives/Button';
import { Chip } from '@/ui/primitives/Chip';
import { StatusDot } from '@/ui/data/StatusDot';
import { useMe } from '@/state/mode';
import { useTelemetry } from '@/state/useTelemetry';
import { ModuleSnapshot } from '../../../../../protocol/gen/ts/messages/modules_pb';
import {
  listModules,
  stageModule,
  confirmModule,
  uninstallModule,
  type ModuleListItem,
  type StageResult,
} from '../modulesApi';
import {
  listRegistered,
  registerModule,
  listRoverDesired,
  unpinRoverModule,
  deleteModule,
  type RegisteredModule,
  type DesiredModule,
  type ConfigFieldSpec,
} from '../proxyModulesApi';
import { ModuleConfigDialog } from '../ModuleConfigDialog';
import styles from './ModulesManagePanel.module.css';

type Props = { roverId?: string };

export function ModulesManagePanel({ roverId = '' }: Props) {
  const me = useMe();
  // me is null until /api/me resolves; isLocal/isProxy stay false meanwhile so
  // neither mode's effect fires against the wrong origin (a premature proxy
  // fetch hits the agent's /api/admin/* — which 404s — and leaves a stale error).
  const isLocal = me?.mode === 'local';
  const isProxy = me?.mode === 'proxy';

  const [modules, setModules] = useState<ModuleListItem[]>([]);
  const [error, setError] = useState<string | null>(null);

  // Live health from the infra.modules snapshot.
  const snapshot = useTelemetry(
    `waypoint.${roverId}.infra.modules`,
    (b) => ModuleSnapshot.fromBinary(b),
  );
  const healthById = new Map<string, boolean>(
    (snapshot?.modules ?? []).map((m) => [m.id, m.healthy]),
  );

  async function refresh() {
    try {
      setModules(await listModules());
      setError(null);
    } catch (e) {
      // List failure is non-fatal — show what we had and note the error.
      setError(String((e as Error).message ?? e));
    }
  }

  // Gate local list fetch behind local mode to avoid 404 HTML on the proxy.
  useEffect(() => { if (isLocal) void refresh(); }, [isLocal]);

  // ── Proxy-mode state ─────────────────────────────────────────────────────
  const [registered, setRegistered] = useState<RegisteredModule[]>([]);
  const [desired, setDesired] = useState<DesiredModule[]>([]);
  const [repoUrl, setRepoUrl] = useState('');
  const [proxyBusy, setProxyBusy] = useState<string | null>(null); // moduleId being enabled, or 'register'

  async function refreshProxy() {
    try {
      const [regs, des] = await Promise.all([listRegistered(), listRoverDesired(roverId)]);
      setRegistered(regs);
      setDesired(des);
      setError(null);
    } catch (e) {
      setError(String((e as Error).message ?? e));
    }
  }

  useEffect(() => { if (isProxy && roverId) void refreshProxy(); }, [isProxy, roverId]);

  // Enabling and reconfiguring are the same POST, so both open the config
  // dialog: a module that needs credentials gets them before it ever starts.
  const [configTarget, setConfigTarget] = useState<{
    moduleId: string;
    version: string;
    configToml: string;
    intent: 'enable' | 'edit';
    fields: ConfigFieldSpec[];
  } | null>(null);

  async function handleDisable(moduleId: string) {
    setProxyBusy(moduleId);
    setError(null);
    try {
      await unpinRoverModule(roverId, moduleId);
      await refreshProxy();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setProxyBusy(null);
    }
  }

  async function handleRemoveFromRegistry(moduleId: string) {
    if (!window.confirm(`Delete ${moduleId} from the registry? The proxy refuses while it is still enabled on any rover.`)) return;
    setProxyBusy(moduleId);
    setError(null);
    try {
      await deleteModule(moduleId);
      await refreshProxy();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setProxyBusy(null);
    }
  }

  async function handleRegister() {
    if (!repoUrl) return;
    setProxyBusy('register');
    setError(null);
    try {
      await registerModule(repoUrl);
      setRepoUrl('');
      await refreshProxy();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setProxyBusy(null);
    }
  }

  // ── Install flow state ───────────────────────────────────────────────────
  const rawRef = useRef<HTMLInputElement>(null);
  const bundleRef = useRef<HTMLInputElement>(null);
  const [staged, setStaged] = useState<StageResult | null>(null);
  const [configToml, setConfigToml] = useState('');
  const [busy, setBusy] = useState(false);

  async function handleVerify() {
    const raw = rawRef.current?.files?.[0];
    const bundle = bundleRef.current?.files?.[0];
    if (!raw || !bundle) return;
    setBusy(true);
    setError(null);
    try {
      const result = await stageModule(raw, bundle);
      setStaged(result);
      setConfigToml('');
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  async function handleConfirm() {
    if (!staged) return;
    setBusy(true);
    setError(null);
    try {
      await confirmModule(staged.stageId, configToml);
      setStaged(null);
      setConfigToml('');
      if (rawRef.current) rawRef.current.value = '';
      if (bundleRef.current) bundleRef.current.value = '';
      await refresh();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  function handleCancelStage() {
    setStaged(null);
    setConfigToml('');
    setError(null);
    if (rawRef.current) rawRef.current.value = '';
    if (bundleRef.current) bundleRef.current.value = '';
  }

  async function handleUninstall(id: string) {
    setBusy(true);
    setError(null);
    try {
      await uninstallModule(id);
      await refresh();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  // Derive health status for display.
  function healthStatus(id: string): 'ok' | 'fault' | 'off' {
    const h = healthById.get(id);
    if (h === undefined) return 'off';
    return h ? 'ok' : 'fault';
  }

  return (
    <div className={styles.layout} data-testid="panel-modules">
      {/* ── LOCAL mode ───────────────────────────────────────────────────── */}
      {isLocal && (
        <>
          {/* Installed modules */}
          <Panel title="INSTALLED MODULES" note={`${modules.length} installed`}>
            {modules.length === 0 ? (
              <p className={styles.empty}>No modules installed.</p>
            ) : (
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Version</th>
                    <th>Origin</th>
                    <th>Health</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {modules.map((m) => (
                    <tr key={m.id}>
                      <td className={styles.idCell}>{m.id}</td>
                      <td className={styles.versionCell}>{m.version || '—'}</td>
                      <td>
                        <Chip tone={m.origin === 'local' ? 'default' : 'caution'}>
                          {m.origin}
                        </Chip>
                      </td>
                      <td>
                        <StatusDot status={healthStatus(m.id)} />
                      </td>
                      <td>
                        {m.origin === 'local' ? (
                          <Button
                            size="sm"
                            variant="danger"
                            disabled={busy}
                            onClick={() => void handleUninstall(m.id)}
                          >
                            Uninstall
                          </Button>
                        ) : (
                          <span className={styles.managedHint}>managed by proxy</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Panel>

          {/* Install panel — LOCAL only */}
          <Panel title="INSTALL MODULE">
            <div className={styles.form}>
              {!staged && (
                <>
                  <div className={styles.fileRow}>
                    <label className={styles.field}>
                      <span className={styles.label}>Module image (.raw)</span>
                      <input
                        ref={rawRef}
                        className={styles.fileInput}
                        type="file"
                        accept=".raw"
                        aria-label="Module image (.raw)"
                      />
                    </label>
                    <label className={styles.field}>
                      <span className={styles.label}>Cosign bundle (.json / .bundle)</span>
                      <input
                        ref={bundleRef}
                        className={styles.fileInput}
                        type="file"
                        accept=".json,.bundle,.cosign"
                        aria-label="Cosign bundle"
                      />
                    </label>
                  </div>
                  <div className={styles.actions}>
                    <Button
                      variant="primary"
                      disabled={busy}
                      onClick={() => void handleVerify()}
                    >
                      {busy ? 'Verifying…' : 'Verify'}
                    </Button>
                  </div>
                </>
              )}

              {staged && (
                <div className={styles.confirmCard} data-testid="confirm-card">
                  <div className={styles.confirmTitle}>Review before installing</div>
                  <div className={styles.confirmMeta}>
                    <div className={styles.confirmRow}>
                      <span className={styles.confirmKey}>Signer SAN</span>
                      <code className={styles.san} data-testid="signer-san">
                        {staged.signerSan}
                      </code>
                    </div>
                    <div className={styles.confirmRow}>
                      <span className={styles.confirmKey}>ID</span>
                      <span className={styles.confirmVal}>{staged.id}</span>
                    </div>
                    <div className={styles.confirmRow}>
                      <span className={styles.confirmKey}>Version</span>
                      <span className={styles.confirmVal}>{staged.version}</span>
                    </div>
                    <div className={styles.confirmRow}>
                      <span className={styles.confirmKey}>Label</span>
                      <span className={styles.confirmVal}>{staged.label}</span>
                    </div>
                    {staged.uiKind && staged.uiKind !== 'none' && (
                      <div className={styles.confirmRow}>
                        <span className={styles.confirmKey}>UI</span>
                        <span className={styles.confirmVal}>
                          {staged.uiKind}
                          {staged.tabId ? ` · ${staged.tabId}` : ''}
                        </span>
                      </div>
                    )}
                    {(staged.permissions ?? []).length > 0 && (
                      <div className={styles.confirmRow}>
                        <span className={styles.confirmKey}>Permissions</span>
                        <div className={styles.permList}>
                          {(staged.permissions ?? []).map((p) => (
                            <Chip key={p} tone="caution">{p}</Chip>
                          ))}
                        </div>
                      </div>
                    )}
                    {(staged.devices ?? []).length > 0 && (
                      <div className={styles.confirmRow}>
                        <span className={styles.confirmKey}>Devices</span>
                        <div className={styles.permList}>
                          {(staged.devices ?? []).map((d) => (
                            <Chip key={d}>{d}</Chip>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>

                  <div className={styles.configSection}>
                    <span className={styles.label}>Config (TOML, optional)</span>
                    <textarea
                      className={styles.textarea}
                      value={configToml}
                      onChange={(e) => setConfigToml(e.target.value)}
                      placeholder="# module-specific config"
                      aria-label="Config TOML"
                    />
                  </div>

                  <div className={styles.confirmActions}>
                    <Button
                      variant="primary"
                      disabled={busy}
                      onClick={() => void handleConfirm()}
                    >
                      {busy ? 'Installing…' : 'Trust & install'}
                    </Button>
                    <Button disabled={busy} onClick={handleCancelStage}>
                      Cancel
                    </Button>
                  </div>
                </div>
              )}
            </div>

            {error !== null && (
              <div className={styles.error} role="alert" data-testid="error-region">
                {error}
              </div>
            )}
          </Panel>
        </>
      )}

      {/* ── PROXY mode ───────────────────────────────────────────────────── */}
      {!isLocal && (
        <>
          {/* Running on this rover */}
          <Panel title="RUNNING ON THIS ROVER" note={`${snapshot?.modules?.length ?? 0} running`}>
            {(snapshot?.modules ?? []).length === 0 ? (
              <p className={styles.empty}>No modules running.</p>
            ) : (
              <table className={styles.table} data-testid="running-table">
                <thead>
                  <tr>
                    <th>Health</th>
                    <th>ID</th>
                    <th>Version</th>
                  </tr>
                </thead>
                <tbody>
                  {(snapshot?.modules ?? []).map((m) => (
                    <tr key={m.id}>
                      <td><StatusDot status={healthStatus(m.id)} /></td>
                      <td className={styles.idCell}>
                        {m.id}
                        {m.label ? <span className={styles.moduleLabel}>{m.label}</span> : null}
                      </td>
                      <td className={styles.versionCell}>{m.version || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Panel>

          {/* Registry */}
          <Panel title="REGISTRY" note={`${registered.length} modules`}>
            {registered.length === 0 ? (
              <p className={styles.empty}>No modules registered.</p>
            ) : (
              <table className={styles.table} data-testid="registry-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th className={styles.thVersion}>Latest</th>
                    <th className={styles.thVersion}>Desired</th>
                    <th className={styles.thActions}></th>
                  </tr>
                </thead>
                <tbody>
                  {registered.map((m) => {
                    const latestVersion = m.versions[0]?.version ?? '';
                    const desiredEntry = desired.find((d) => d.moduleId === m.moduleId);
                    const isEnabling = proxyBusy === m.moduleId;
                    return (
                      <tr key={m.moduleId}>
                        <td className={styles.idCell}>
                          {m.moduleId}
                          {m.displayName ? <span className={styles.moduleLabel}>{m.displayName}</span> : null}
                        </td>
                        <td className={styles.versionCell}>{latestVersion || '—'}</td>
                        <td>
                          {desiredEntry ? (
                            <Chip>{desiredEntry.version}</Chip>
                          ) : (
                            <span className={styles.managedHint}>—</span>
                          )}
                        </td>
                        <td>
                          <div className={styles.rowActions}>
                            {desiredEntry ? (
                              <>
                                <Button
                                  size="sm"
                                  disabled={isEnabling}
                                  onClick={() => void handleDisable(m.moduleId)}
                                  data-testid={`disable-${m.moduleId}`}
                                >
                                  Disable on this rover
                                </Button>
                                <button
                                  type="button"
                                  className={styles.configBtn}
                                  disabled={isEnabling}
                                  title={`Configure ${m.moduleId}`}
                                  aria-label={`Configure ${m.moduleId}`}
                                  onClick={() => setConfigTarget({
                                    moduleId: m.moduleId,
                                    version: desiredEntry.version,
                                    configToml: desiredEntry.configToml,
                                    intent: 'edit',
                                    // The pinned version's schema, which an older
                                    // pin may declare differently from the latest.
                                    fields: m.versions.find((v) => v.version === desiredEntry.version)?.configFields ?? [],
                                  })}
                                  data-testid={`configure-${m.moduleId}`}
                                >
                                  <Settings size={13} />
                                </button>
                              </>
                            ) : (
                              <Button
                                size="sm"
                                variant="primary"
                                disabled={isEnabling || !latestVersion}
                                onClick={() => setConfigTarget({
                                  moduleId: m.moduleId,
                                  version: latestVersion,
                                  configToml: '',
                                  intent: 'enable',
                                  fields: m.versions[0]?.configFields ?? [],
                                })}
                                data-testid={`enable-${m.moduleId}`}
                              >
                                Enable on this rover
                              </Button>
                            )}
                            <button
                              type="button"
                              className={styles.removeLink}
                              disabled={isEnabling}
                              onClick={() => void handleRemoveFromRegistry(m.moduleId)}
                              data-testid={`remove-${m.moduleId}`}
                            >
                              remove
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </Panel>

          {/* Register module */}
          <Panel title="REGISTER MODULE">
            <div className={styles.form}>
              <div className={styles.registerRow}>
                <label className={styles.field}>
                  <span className={styles.label}>Source repo URL</span>
                  <input
                    className={styles.textInput}
                    type="text"
                    value={repoUrl}
                    onChange={(e) => setRepoUrl(e.target.value)}
                    placeholder="https://github.com/owner/repo"
                    aria-label="Source repo URL"
                  />
                </label>
              </div>
              <p className={styles.registerHint}>
                Registration pulls and verifies the module's latest GitHub release.
              </p>
              <div className={styles.actions}>
                <Button
                  variant="primary"
                  disabled={proxyBusy === 'register' || !repoUrl}
                  onClick={() => void handleRegister()}
                  data-testid="register-btn"
                >
                  {proxyBusy === 'register' ? 'Registering…' : 'Register'}
                </Button>
              </div>
            </div>

            {error !== null && (
              <div className={styles.error} role="alert" data-testid="error-region">
                {error}
              </div>
            )}
          </Panel>
        </>
      )}

      {configTarget && (
        <ModuleConfigDialog
          open
          roverId={roverId}
          moduleId={configTarget.moduleId}
          version={configTarget.version}
          configToml={configTarget.configToml}
          fields={configTarget.fields}
          intent={configTarget.intent}
          onSaved={() => void refreshProxy()}
          onClose={() => setConfigTarget(null)}
        />
      )}
    </div>
  );
}
