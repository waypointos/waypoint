// dashboard/src/ui/forms/ApplyImageDialog.tsx
//
// POST /api/rovers/{id}/apply-image — admin-only OTA "apply update" dialog.
import { useState } from 'react';
import { Dialog } from '../primitives/Dialog';
import { Button } from '../primitives/Button';
import styles from './ApplyImageDialog.module.css';

type Props = {
  roverId: string;
  open: boolean;
  onClose: () => void;
};

export function ApplyImageDialog({ roverId, open, onClose }: Props) {
  const [url, setUrl]         = useState('');
  const [sha256, setSha256]   = useState('');
  const [version, setVersion] = useState('');
  const [busy, setBusy]       = useState(false);
  const [errMsg, setErr]      = useState('');

  function reset() {
    setUrl(''); setSha256(''); setVersion(''); setBusy(false); setErr('');
  }

  function handleClose() {
    reset();
    onClose();
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr('');
    try {
      const r = await fetch(`/api/rovers/${roverId}/apply-image`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: url.trim(),
          expectedSha256: sha256.trim(),
          version: version.trim(),
        }),
      });
      if (r.status === 202) {
        handleClose();
        return;
      }
      const text = await r.text();
      setErr(text || `HTTP ${r.status}`);
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : 'network error');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onClose={handleClose} title={`Apply image to ${roverId}`}>
      <form className={styles.form} onSubmit={handleSubmit}>
        <label className={styles.field}>
          <span className={styles.label}>Release URL (.swu)</span>
          <input
            className={styles.input}
            type="url"
            placeholder="https://github.com/…/waypoint-prod-0.5.1.swu"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            required
            autoFocus
          />
        </label>
        <label className={styles.field}>
          <span className={styles.label}>Expected SHA-256 (optional)</span>
          <input
            className={styles.input}
            type="text"
            placeholder="64-hex-char digest"
            value={sha256}
            onChange={(e) => setSha256(e.target.value)}
          />
        </label>
        <label className={styles.field}>
          <span className={styles.label}>Version label (optional)</span>
          <input
            className={styles.input}
            type="text"
            placeholder="0.5.1"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
          />
        </label>

        <div className={styles.warn}>
          The rover will reboot. Expect ~30 seconds of downtime.
        </div>

        {errMsg && <div className={styles.err}>{errMsg}</div>}

        <div className={styles.actions}>
          <Button type="button" onClick={handleClose} disabled={busy}>Cancel</Button>
          <Button variant="primary" type="submit" disabled={!url.trim() || busy}>
            {busy ? 'Applying…' : 'Apply'}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
