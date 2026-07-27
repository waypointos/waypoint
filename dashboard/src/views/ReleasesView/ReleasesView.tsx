import { useCallback, useEffect, useState } from 'react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Button } from '@/ui/primitives/Button';
import { Chip } from '@/ui/primitives/Chip';
import { VersionDiffModal } from '@/ui/forms/VersionDiffModal';
import { ApplyImageDialog } from '@/ui/forms/ApplyImageDialog';
import { RegisterImageSourceDialog } from './RegisterImageSourceDialog';
import { listReleasesFleet, checkImageUpdates, applyImage, type ReleaseRover } from './releasesApi';
import styles from './ReleasesView.module.css';

export function ReleasesView() {
  const [fleet, setFleet] = useState<ReleaseRover[]>([]);
  const [reviewFor, setReviewFor] = useState<string | null>(null);
  const [advancedFor, setAdvancedFor] = useState<string | null>(null);
  const [registering, setRegistering] = useState(false);
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [applyErr, setApplyErr] = useState('');
  const [err, setErr] = useState('');

  const refresh = useCallback(async () => {
    try {
      setFleet(await listReleasesFleet());
    } catch (e) {
      setErr(String((e as Error).message ?? e));
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function check() {
    setChecking(true);
    setErr('');
    try {
      await checkImageUpdates();
      await refresh();
    } catch (e) {
      setErr(String((e as Error).message ?? e));
    } finally {
      setChecking(false);
    }
  }

  const review = fleet.find((f) => f.roverId === reviewFor) ?? null;

  async function apply(rover: ReleaseRover) {
    setApplying(true);
    setApplyErr('');
    try {
      await applyImage(rover.roverId, rover.swuUrl, rover.swuSha256, rover.latestVersion);
      setReviewFor(null);
      await refresh();
    } catch (e) {
      setApplyErr(String((e as Error).message ?? e));
    } finally {
      setApplying(false);
    }
  }

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// RELEASES</h1>
            <div className={styles.sub}>OS image releases</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <div className={styles.toolbar}>
            <Button onClick={check} disabled={checking}>{checking ? 'Checking…' : 'Check for updates'}</Button>
            <Button variant="primary" onClick={() => setRegistering(true)}>+ Register source</Button>
          </div>
          {err && <div className={styles.err}>{err}</div>}
          {fleet.map((rv) => (
        <div key={rv.roverId} className={styles.row}>
          <div>
            <div className={styles.name}>{rv.name}</div>
            <div className={styles.meta}>{rv.channel} channel</div>
          </div>
          <div className={styles.right}>
            <span className={styles.cur}>{rv.currentVersion || '—'}</span>
            <span className={styles.arrow} aria-hidden>→</span>
            <span className={styles.new}>{rv.latestVersion || '—'}</span>
            {rv.updateAvailable
              ? <Chip tone="caution" dot>update</Chip>
              : <Chip dot>up to date</Chip>}
            {rv.updateAvailable && <Button size="sm" variant="primary" onClick={() => { setApplyErr(''); setReviewFor(rv.roverId); }}>Review</Button>}
            <Button size="sm" onClick={() => setAdvancedFor(rv.roverId)}>Advanced…</Button>
          </div>
        </div>
          ))}
        </section>
      </main>

      {review && (
        <VersionDiffModal
          open
          title={`Update OS image · ${review.name}`}
          currentVersion={review.currentVersion || '—'}
          newVersion={review.latestVersion}
          meta={`${review.channel} channel · cosign-verified on the rover · SHA256 pinned`}
          changelogMarkdown={review.releaseNotesMd}
          warning={<span>⚠ The rover reboots into the new partition (~90s offline). A/B rollback applies if boot fails.</span>}
          applyLabel="Apply & reboot"
          busy={applying}
          errorMsg={applyErr}
          onApply={() => void apply(review)}
          onClose={() => { setApplyErr(''); setReviewFor(null); }}
        />
      )}
      {advancedFor && (
        <ApplyImageDialog roverId={advancedFor} open onClose={() => setAdvancedFor(null)} />
      )}
      <RegisterImageSourceDialog open={registering} onClose={() => setRegistering(false)} onDone={refresh} />
    </div>
  );
}
