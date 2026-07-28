// Per-rover module configuration (POST /api/admin/rovers/{id}/modules/{moduleId}).
// Renders a form from the module's manifest [[config.fields]] schema, and falls
// back to raw TOML for a module that declares none. moduleConfigToml handles the
// [modules_config.<id>] table the agent decodes.
import { useEffect, useMemo, useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { Dialog } from '@/ui/primitives/Dialog';
import { Button } from '@/ui/primitives/Button';
import {
  unwrapModuleConfig, wrapModuleConfig, parseFlatConfig, buildFlatConfig,
} from '@/lib/moduleConfigToml';
import { setRoverDesired, type ConfigFieldSpec } from './proxyModulesApi';
import styles from './ModuleConfigDialog.module.css';

type Props = {
  open: boolean;
  roverId: string;
  moduleId: string;
  /** Desired version to keep pinned; the POST would otherwise blank it. */
  version: string;
  configToml: string;
  /** Manifest [[config.fields]]; empty renders the raw TOML editor. */
  fields?: readonly ConfigFieldSpec[];
  /** Copy switches when the module is not enabled on this rover yet. */
  intent?: 'edit' | 'enable';
  onSaved: () => void;
  onClose: () => void;
};

// A declared type an older dashboard predates renders as text rather than
// disappearing, which is why the manifest carries a string and not an enum.
function inputType(type: string): string {
  switch (type) {
    case 'url': return 'url';
    case 'password': return 'password';
    case 'number': return 'number';
    default: return 'text';
  }
}

export function ModuleConfigDialog({
  open, roverId, moduleId, version, configToml, fields = [], intent = 'edit', onSaved, onClose,
}: Props) {
  const schema = fields.length > 0 ? fields : null;
  const [values, setValues] = useState<Record<string, string>>({});
  const [text, setText] = useState('');
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const flat = useMemo(() => unwrapModuleConfig(configToml, moduleId), [configToml, moduleId]);

  useEffect(() => {
    if (!open) return;
    setValues(parseFlatConfig(flat));
    setText(flat);
    setRevealed({});
    setError('');
  }, [open, flat]);

  const missing = (schema ?? []).filter((f) => f.required && (values[f.key] ?? '').trim() === '');

  async function save() {
    setBusy(true);
    setError('');
    try {
      const body = schema
        ? buildFlatConfig(values, schema.map((f) => ({ key: f.key, type: f.type })), flat)
        : text;
      await setRoverDesired(roverId, moduleId, version, wrapModuleConfig(body, moduleId));
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

      {schema ? (
        <div className={styles.fields}>
          {schema.map((f) => {
            const isSecret = f.type === 'password';
            const show = revealed[f.key] === true;
            return (
              <label key={f.key} className={styles.field}>
                <span className={styles.label}>
                  {f.label || f.key}
                  {f.required && <span className={styles.req} aria-hidden> *</span>}
                </span>
                <span className={styles.inputWrap}>
                  <input
                    className={styles.input}
                    type={isSecret && show ? 'text' : inputType(f.type)}
                    value={values[f.key] ?? ''}
                    onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
                    placeholder={f.defaultValue}
                    aria-label={f.label || f.key}
                    spellCheck={false}
                    autoComplete={isSecret ? 'new-password' : 'off'}
                  />
                  {isSecret && (
                    <button
                      type="button"
                      className={styles.reveal}
                      onClick={() => setRevealed((r) => ({ ...r, [f.key]: !show }))}
                      aria-label={show ? `Hide ${f.label || f.key}` : `Show ${f.label || f.key}`}
                    >
                      {show ? <EyeOff size={13} /> : <Eye size={13} />}
                    </button>
                  )}
                </span>
                {f.help && <span className={styles.help}>{f.help}</span>}
              </label>
            );
          })}
        </div>
      ) : (
        <>
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
            This module declares no config schema, so its keys are free-form. Read its docs for
            the shape.
          </p>
        </>
      )}

      <p className={styles.hint}>
        Values are stored on the proxy and delivered only to this rover. The module restarts to
        pick them up. A field left blank keeps the module's own default.
      </p>
      {error && <div className={styles.err} role="alert">{error}</div>}
      <div className={styles.actions}>
        <Button
          variant="primary"
          type="button"
          onClick={() => void save()}
          disabled={busy || missing.length > 0}
          data-testid="save-module-config"
        >
          {busy ? 'Saving…' : intent === 'enable' ? 'Enable on this rover' : 'Save'}
        </Button>
        <Button type="button" onClick={onClose} disabled={busy}>Cancel</Button>
        {missing.length > 0 && (
          <span className={styles.blocked}>
            {missing.map((f) => f.label || f.key).join(', ')} required
          </span>
        )}
      </div>
    </Dialog>
  );
}
