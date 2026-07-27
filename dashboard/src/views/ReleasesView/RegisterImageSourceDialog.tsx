import { useState } from 'react';
import { Dialog } from '@/ui/primitives/Dialog';
import { Button } from '@/ui/primitives/Button';
import { registerImageSource } from './releasesApi';
import styles from './RegisterImageSourceDialog.module.css';

type Props = { open: boolean; onClose: () => void; onDone: () => void };

export function RegisterImageSourceDialog({ open, onClose, onDone }: Props) {
  const [repoUrl, setRepoUrl] = useState('');
  const [channel, setChannel] = useState('prod');
  const [visibility, setVisibility] = useState('public');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  function reset() {
    setRepoUrl('');
    setChannel('prod');
    setVisibility('public');
    setToken('');
    setErr('');
  }

  function handleClose() {
    reset();
    onClose();
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr('');
    try {
      await registerImageSource(repoUrl.trim(), channel, visibility, visibility === 'private' ? token.trim() : '');
      onDone();
      reset();
      onClose();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : 'failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onClose={handleClose} title="Register image source">
      <form className={styles.form} onSubmit={submit}>
        <label className={styles.field}>
          <span className={styles.label}>Repo URL</span>
          <input
            className={styles.input}
            type="url"
            value={repoUrl}
            aria-label="Repo URL"
            onChange={(e) => setRepoUrl(e.target.value)}
            placeholder="https://github.com/acme/waypoint-image"
            required
            autoFocus
          />
        </label>
        <label className={styles.field}>
          <span className={styles.label}>Channel</span>
          <select
            className={styles.input}
            value={channel}
            aria-label="Channel"
            onChange={(e) => setChannel(e.target.value)}
          >
            <option value="prod">prod</option>
            <option value="dev">dev</option>
          </select>
        </label>
        <label className={styles.field}>
          <span className={styles.label}>Visibility</span>
          <select
            className={styles.input}
            value={visibility}
            aria-label="Visibility"
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
              aria-label="GitHub token"
              onChange={(e) => setToken(e.target.value)}
              placeholder="ghp_…"
            />
          </label>
        )}
        {err && <div className={styles.err}>{err}</div>}
        <div className={styles.actions}>
          <Button type="button" onClick={handleClose} disabled={busy}>Cancel</Button>
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
