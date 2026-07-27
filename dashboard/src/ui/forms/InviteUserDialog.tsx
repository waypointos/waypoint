// dashboard/src/ui/forms/InviteUserDialog.tsx
//
// POST /api/users/invite — admin-only WorkOS invitation flow.
import { useState } from 'react';
import { Copy } from 'lucide-react';
import { Dialog } from '../primitives/Dialog';
import { Button } from '../primitives/Button';
import { inviteUser, type InviteResult } from '@/state/useAdminUsers';
import styles from './InviteUserDialog.module.css';

type Props = {
  open: boolean;
  onClose: () => void;
  /** Fired after a successful invite so the parent can refresh its list. */
  onInvited?: () => void;
};

type Phase = 'form' | 'success' | 'error';

export function InviteUserDialog({ open, onClose, onInvited }: Props) {
  const [email, setEmail] = useState('');
  const [phase, setPhase] = useState<Phase>('form');
  const [result, setResult] = useState<InviteResult | null>(null);
  const [errMsg, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);

  function reset() {
    setEmail('');
    setPhase('form');
    setResult(null);
    setErr('');
    setBusy(false);
    setCopied(false);
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
      const r = await inviteUser(email.trim());
      setResult(r);
      setPhase('success');
      onInvited?.();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : 'network error');
      setPhase('error');
    } finally {
      setBusy(false);
    }
  }

  async function handleCopy() {
    if (!result?.acceptUrl) return;
    await navigator.clipboard.writeText(result.acceptUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Dialog open={open} onClose={handleClose} title="Invite user">
      {phase === 'form' || phase === 'error' ? (
        <form className={styles.form} onSubmit={handleSubmit}>
          <label className={styles.field}>
            <span className={styles.label}>Email</span>
            <input
              className={styles.input}
              type="email"
              placeholder="alice@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoFocus
            />
          </label>

          {phase === 'error' && errMsg && (
            <div className={styles.err}>{errMsg}</div>
          )}

          <div className={styles.actions}>
            <Button type="button" onClick={handleClose}>Cancel</Button>
            <Button variant="primary" type="submit" disabled={busy}>
              {busy ? 'Inviting…' : 'Invite'}
            </Button>
          </div>
        </form>
      ) : (
        <div className={styles.success}>
          {result?.emailSent ? (
            <div className={styles.successNote}>
              Invitation email sent to{' '}
              <code className={styles.path}>{result.email}</code>.
            </div>
          ) : (
            <div className={styles.successNote}>
              Invite created for{' '}
              <code className={styles.path}>{result?.email}</code>. Share the
              link below manually — no email was dispatched.
            </div>
          )}

          {result?.acceptUrl ? (
            <div className={styles.linkWrap}>
              <pre className={styles.linkPre}>{result.acceptUrl}</pre>
              <button
                className={styles.copyBtn}
                onClick={handleCopy}
                type="button"
              >
                <Copy size={13} />
                {copied ? 'Copied!' : 'Copy'}
              </button>
            </div>
          ) : (
            <div className={styles.naRow}>
              <span className={styles.naLabel}>N/A</span>
              <span className={styles.naHint}>
                Accept URL not returned by the proxy.
              </span>
            </div>
          )}

          <div className={styles.actions}>
            <Button onClick={handleClose}>Done</Button>
          </div>
        </div>
      )}
    </Dialog>
  );
}
