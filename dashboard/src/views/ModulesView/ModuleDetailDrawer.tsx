import { useEffect, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { Drawer } from '@/ui/primitives/Drawer';
import { Button } from '@/ui/primitives/Button';
import { Chip } from '@/ui/primitives/Chip';
import { VersionDiffModal } from '@/ui/forms/VersionDiffModal';
import { useFleet } from '@/state/useFleet';
import {
  getModuleReadme, deployModule, checkModuleUpdates, unpinRoverModule, deleteModule,
  type MarketplaceModule,
} from '@/views/RoverView/proxyModulesApi';
import styles from './ModuleDetailDrawer.module.css';

type Props = {
  module: MarketplaceModule;
  open: boolean;
  onClose: () => void;
  onChanged: () => void;
};

export function ModuleDetailDrawer({ module, open, onClose, onChanged }: Props) {
  const { rovers } = useFleet();
  const [readme, setReadme] = useState('');
  const [busy, setBusy] = useState(false);
  const [updateFor, setUpdateFor] = useState<string | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open && module.hasReadme) void getModuleReadme(module.moduleId).then(setReadme);
  }, [open, module.moduleId, module.hasReadme]);

  const stateByRover = new Map(module.rovers.map((r) => [r.roverId, r]));

  async function deployOne(roverId: string, version: string, autoUpdate: boolean) {
    setBusy(true);
    try {
      await deployModule(module.moduleId, [{ roverId, version, autoUpdate, configToml: '' }]);
      onChanged();
    } finally {
      setBusy(false);
      setUpdateFor(null);
    }
  }

  async function check() {
    setBusy(true);
    try {
      await checkModuleUpdates(module.moduleId);
      onChanged();
    } finally {
      setBusy(false);
    }
  }

  async function removeFrom(roverId: string) {
    setBusy(true);
    setError('');
    try {
      await unpinRoverModule(roverId, module.moduleId);
      onChanged();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  async function removeModule() {
    if (!window.confirm(`Delete ${module.displayName} from the registry? Its versions and signing-identity pin are forgotten.`)) return;
    setBusy(true);
    setError('');
    try {
      await deleteModule(module.moduleId);
      onChanged();
      onClose();
    } catch (e) {
      setError(String((e as Error).message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Drawer
        open={open}
        onClose={onClose}
        aria-label={`Module ${module.displayName}`}
        footer={
          <div className={styles.foot}>
            <Button onClick={removeModule} disabled={busy} data-testid="delete-module">Delete module</Button>
            <Button onClick={() => window.open(module.sourceRepoUrl, '_blank')}>Open repo</Button>
            <Button variant="primary" onClick={check} disabled={busy}>Check for updates</Button>
          </div>
        }
      >
        <div className={styles.head}>
          <div className={styles.title}>{module.displayName}</div>
          <div className={styles.repo}>
            {module.sourceRepoUrl.replace('https://github.com/', '')} · {module.repoVisibility} · latest v{module.latestVersion}
          </div>
        </div>

        {error && <div className={styles.error} role="alert">{error}</div>}

        <div className={styles.label}>Readme</div>
        <div className={styles.readme}><ReactMarkdown>{readme}</ReactMarkdown></div>

        <div className={styles.label}>Deploy to rovers</div>
        {rovers.map((rv) => {
          const st = stateByRover.get(rv.id);
          if (st) {
            return (
              <div key={rv.id} className={styles.rover}>
                <span>{rv.name}<span className={styles.rver}> v{st.desiredVersion}</span></span>
                <span className={styles.right}>
                  {st.updateAvailable && (
                    <button type="button" className={styles.updlink} disabled={busy} onClick={() => setUpdateFor(rv.id)}>
                      update → {module.latestVersion}
                    </button>
                  )}
                  <Chip tone={st.autoUpdate ? 'default' : 'caution'}>{st.autoUpdate ? 'auto' : 'pinned'}</Chip>
                  <button
                    type="button"
                    className={styles.updlink}
                    disabled={busy}
                    onClick={() => void removeFrom(rv.id)}
                    data-testid={`remove-${rv.id}`}
                  >
                    remove
                  </button>
                </span>
              </div>
            );
          }
          return (
            <div key={rv.id} className={styles.rover}>
              <span>{rv.name}<span className={styles.rver}> not installed</span></span>
              <Button size="sm" disabled={busy} onClick={() => deployOne(rv.id, module.latestVersion, false)}>
                Install {module.latestVersion}
              </Button>
            </div>
          );
        })}
      </Drawer>

      {updateFor && (
        <VersionDiffModal
          open
          title={`Update ${module.displayName} on ${updateFor}`}
          currentVersion={stateByRover.get(updateFor)?.desiredVersion ?? ''}
          newVersion={module.latestVersion}
          meta={`${module.repoVisibility} repo · cosign-verified`}
          changelogMarkdown=""
          applyLabel="Apply update"
          busy={busy}
          onApply={() => deployOne(updateFor, module.latestVersion, stateByRover.get(updateFor)?.autoUpdate ?? false)}
          onClose={() => setUpdateFor(null)}
        />
      )}
    </>
  );
}
