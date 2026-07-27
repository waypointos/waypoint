// Per-rover module configuration (POST /api/admin/rovers/{id}/modules/{moduleId}).
// The operator types the module's own flat keys; moduleConfigToml handles the
// [modules_config.<id>] table the agent decodes.
import { useEffect, useState } from 'react';
import { Dialog } from '@/ui/primitives/Dialog';
import { Button } from '@/ui/primitives/Button';
import { unwrapModuleConfig, wrapModuleConfig } from '@/lib/moduleConfigToml';
import { setRoverDesired } from './proxyModulesApi';
import styles from './ModuleConfigDialog.module.css';

type Props = {
  open: boolean;
  roverId: string;
  moduleId: string;
  /** Desired version to keep pinned; the POST would otherwise blank it. */
  version: string;
  configToml: string;
  /** Copy switches when the module is not enabled on this rover yet. */
  intent?: 'edit' | 'enable';
  onSaved: () => void;
  onClose: () => void;
};

export function ModuleConfigDialog({
  open, roverId, moduleId, version, configToml, intent = 'edit', onSaved, onClose,
}: Props) {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setText(unwrapModuleConfig(configToml, moduleId));
    setError('');
  }, [open, configToml, moduleId]);

  async function save() {
    setBusy(true);
    setError('');
    try {
      await setRoverDesired(roverId, moduleId, version, wrapModuleConfig(text, moduleId));
      onSaved();
      onClose();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onClose={onClose} title={`${moduleId} configuration`}>
      <div className={styles.meta}>
        <span className={styles.key}>Version</span>
        <span className={styles.val}>{version || '—'}</span>
      </div>
      <label className={styles.field}>
        <span className={styles.label}>Config (TOML)</span>
        <textarea
          className={styles.textarea}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={'host = "https://192.168.105.1"\npassword = "…"'}
          aria-label="Module config TOML"
          spellCheck={false}
          autoComplete="off"
        />
      </label>
      <p className={styles.hint}>
        Keys are defined by the module; read its docs for the schema. Values are stored on the
        proxy and delivered only to this rover. The module restarts to pick them up.
      </p>
      {error && <div className={styles.err} role="alert">{error}</div>}
      <div className={styles.actions}>
        <Button type="button" onClick={onClose} disabled={busy}>Cancel</Button>
        <Button
          variant="primary"
          type="button"
          onClick={() => void save()}
          disabled={busy}
          data-testid="save-module-config"
        >
          {busy ? 'Saving…' : intent === 'enable' ? 'Enable on this rover' : 'Save'}
        </Button>
      </div>
    </Dialog>
  );
}
