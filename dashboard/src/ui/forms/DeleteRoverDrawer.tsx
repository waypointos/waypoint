// dashboard/src/ui/forms/DeleteRoverDrawer.tsx
//
// DELETE /api/rovers/{id} — admin-only confirmation drawer. Matches the
// EnrollRoverWizard pattern: eyebrow + title + subtitle in the header, a
// detail pane in the middle, and a footer with cancel/destructive actions.
// The backend revokes the rover's NATS account before dropping the DB row.
import { useEffect, useState } from 'react';
import { Drawer } from '@/ui/primitives/Drawer';
import { Button } from '@/ui/primitives/Button';
import styles from './DeleteRoverDrawer.module.css';

type Props = {
  open: boolean;
  /** Rover being deleted. `null` is treated as closed (renders nothing). */
  rover: { id: string; name: string } | null;
  onClose: () => void;
  /** Called after a successful delete so callers can refresh state. */
  onDeleted?: () => void;
};

export function DeleteRoverDrawer({ open, rover, onClose, onDeleted }: Props) {
  const [busy, setBusy] = useState(false);
  const [err, setErr]   = useState('');

  // Clear transient state every time the drawer reopens.
  useEffect(() => {
    if (open) {
      setBusy(false);
      setErr('');
    }
  }, [open]);

  function handleClose() {
    if (busy) return;
    onClose();
  }

  async function handleDelete() {
    if (!rover) return;
    setBusy(true);
    setErr('');
    try {
      const r = await fetch(`/api/rovers/${encodeURIComponent(rover.id)}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!r.ok) {
        setErr((await r.text()) || `delete failed (${r.status})`);
        return;
      }
      onDeleted?.();
      onClose();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : 'network error');
    } finally {
      setBusy(false);
    }
  }

  const isOpen = open && !!rover;

  return (
    <Drawer
      open={isOpen}
      onClose={handleClose}
      aria-label="Delete rover"
      footer={
        <div className={styles.actions}>
          <Button type="button" onClick={handleClose} disabled={busy}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="danger"
            onClick={handleDelete}
            disabled={busy}
          >
            {busy ? 'Deleting…' : 'Delete rover'}
          </Button>
        </div>
      }
    >
      <div className={styles.head}>
        <div className={styles.eyebrow}>DESTRUCTIVE</div>
        <div className={styles.title}>Delete {rover?.id}</div>
        <div className={styles.subtitle}>
          {rover?.name
            ? <>This will permanently remove <strong>{rover.name}</strong> from the fleet.</>
            : <>This will permanently remove the rover from the fleet.</>}
        </div>
      </div>

      <div className={styles.pane}>
        <section className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.bk}>[</span>CONSEQUENCES<span className={styles.bk}>]</span>
          </div>
          <ul className={styles.list}>
            <li>
              <span className={styles.bullet}>—</span>
              <span>NATS account is revoked immediately; the rover loses bus access mid-flight.</span>
            </li>
            <li>
              <span className={styles.bullet}>—</span>
              <span>All user access grants for this rover are removed.</span>
            </li>
            <li>
              <span className={styles.bullet}>—</span>
              <span>Image apply history and active alerts are deleted.</span>
            </li>
            <li>
              <span className={styles.bullet}>—</span>
              <span>Audit log entries are preserved (rover id stays referenceable).</span>
            </li>
          </ul>
        </section>

        {err ? <div className={styles.err}>{err}</div> : null}
      </div>
    </Drawer>
  );
}
