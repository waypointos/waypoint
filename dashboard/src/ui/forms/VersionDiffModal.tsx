import ReactMarkdown from 'react-markdown';
import { Dialog } from '@/ui/primitives/Dialog';
import { Button } from '@/ui/primitives/Button';
import styles from './VersionDiffModal.module.css';

type Props = {
  open: boolean;
  title: string;
  currentVersion: string;
  newVersion: string;
  /** Single-line verification/signer metadata under the version flow. */
  meta?: string;
  /** Raw markdown from the release notes. */
  changelogMarkdown: string;
  /** Optional notice (e.g. the image reboot warning). */
  warning?: React.ReactNode;
  applyLabel: string;
  busy?: boolean;
  errorMsg?: string;
  onApply: () => void;
  onClose: () => void;
};

export function VersionDiffModal({
  open, title, currentVersion, newVersion, meta, changelogMarkdown, warning,
  applyLabel, busy, errorMsg, onApply, onClose,
}: Props) {
  return (
    <Dialog open={open} onClose={onClose} title={title}>
      <div className={styles.flow}>
        <div className={styles.vbox}>
          <div className={styles.cap}>Current</div>
          <div className={styles.ver}>{currentVersion}</div>
        </div>
        <div className={styles.arrow} aria-hidden>→</div>
        <div className={`${styles.vbox} ${styles.new}`}>
          <div className={styles.cap}>New</div>
          <div className={styles.ver}>{newVersion}</div>
        </div>
      </div>
      {meta && <div className={styles.meta}>{meta}</div>}
      {changelogMarkdown && (
        <div className={styles.changelog}>
          <ReactMarkdown>{changelogMarkdown}</ReactMarkdown>
        </div>
      )}
      {warning && <div className={styles.warn}>{warning}</div>}
      {errorMsg && <div className={styles.err}>{errorMsg}</div>}
      <div className={styles.actions}>
        <Button type="button" onClick={onClose} disabled={busy}>Cancel</Button>
        <Button variant="primary" type="button" onClick={onApply} disabled={busy}>
          {busy ? 'Applying…' : applyLabel}
        </Button>
      </div>
    </Dialog>
  );
}
