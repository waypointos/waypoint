// Host chrome around a module's own tab content: a settings affordance that
// edits the module's per-rover config. Proxy-only — the agent has no
// config-update endpoint, so in local mode config is install-time input.
import { useState } from 'react';
import { Settings } from 'lucide-react';
import { useMe } from '@/state/mode';
import { ModuleConfigDialog } from './ModuleConfigDialog';
import { listRoverDesired } from './proxyModulesApi';
import styles from './ModuleTabFrame.module.css';

type Props = {
  moduleId: string;
  roverId: string;
  children: React.ReactNode;
};

export function ModuleTabFrame({ moduleId, roverId, children }: Props) {
  const me = useMe();
  const [open, setOpen] = useState(false);
  const [version, setVersion] = useState('');
  const [configToml, setConfigToml] = useState('');
  const [error, setError] = useState('');
  const canConfigure = me?.mode === 'proxy' && me?.isAdmin === true && roverId !== '';

  async function openConfig() {
    setError('');
    try {
      const desired = (await listRoverDesired(roverId)).find((d) => d.moduleId === moduleId);
      // Saving pins the version alongside the config, so without a desired row
      // there is nothing to pin and the save would unpin the module instead.
      if (!desired?.version) {
        setError(`${moduleId} is not enabled on this rover through the proxy; configure it from the MODULES tab.`);
        return;
      }
      setVersion(desired.version);
      setConfigToml(desired.configToml);
      setOpen(true);
    } catch (e) {
      setError(String((e as Error).message ?? e));
    }
  }

  return (
    <div className={styles.frame}>
      {canConfigure && (
        <div className={styles.chrome}>
          <button
            type="button"
            className={styles.gear}
            onClick={() => void openConfig()}
            title={`Configure ${moduleId}`}
            aria-label={`Configure ${moduleId}`}
            data-testid={`configure-${moduleId}`}
          >
            <Settings size={14} />
          </button>
        </div>
      )}
      {error && <div className={styles.err} role="alert">{error}</div>}
      {children}
      {canConfigure && (
        <ModuleConfigDialog
          open={open}
          roverId={roverId}
          moduleId={moduleId}
          version={version}
          configToml={configToml}
          onSaved={() => undefined}
          onClose={() => setOpen(false)}
        />
      )}
    </div>
  );
}
