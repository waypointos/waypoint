import { useState } from 'react';
import { Dialog } from '@/ui/primitives/Dialog';
import { Button } from '@/ui/primitives/Button';
import { registerModule } from '@/views/RoverView/proxyModulesApi';
import styles from './RegisterModuleDialog.module.css';

type Props = { open: boolean; onClose: () => void; onDone: () => void };

export function RegisterModuleDialog({ open, onClose, onDone }: Props) {
  const [repoUrl, setRepoUrl] = useState('');
  const [visibility, setVisibility] = useState('public');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr('');
    try {
      await registerModule(repoUrl.trim(), visibility, visibility === 'private' ? token.trim() : '');
      onDone();
      onClose();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : 'failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onClose={onClose} title="Register module">
      <form className={styles.form} onSubmit={submit}>
        <label className={styles.field}>
          <span className={styles.label}>Repo URL</span>
          <input
            className={styles.input}
            type="url"
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
            placeholder="https://github.com/acme/wp-power"
            required
            autoFocus
          />
        </label>
        <label className={styles.field}>
          <span className={styles.label}>Visibility</span>
          <select
            className={styles.input}
            value={visibility}
            onChange={(e) => setVisibility(e.target.value)}
          >
            <option value="public">public</option>
            <option value="private">private</option>
          </select>
        </label>
        {visibility === 'private' && (
          <label className={styles.field}>
            <span className={styles.label}>GitHub token</span>
            <input
              className={styles.input}
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="ghp_…"
            />
          </label>
        )}
        {err && <div className={styles.err}>{err}</div>}
        <div className={styles.actions}>
          <Button type="button" onClick={onClose} disabled={busy}>Cancel</Button>
          <Button
            variant="primary"
            type="submit"
            disabled={busy || !repoUrl.trim()}
          >
            {busy ? 'Registering…' : 'Register'}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
