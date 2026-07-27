import { Chip } from '@/ui/primitives/Chip';
import { BracketCorners } from '@/ui/primitives/BracketCorners';
import type { MarketplaceModule } from '@/views/RoverView/proxyModulesApi';
import styles from './ModuleCard.module.css';

type Props = { module: MarketplaceModule; onOpen: (moduleId: string) => void };

export function ModuleCard({ module, onOpen }: Props) {
  const behind = module.rovers.filter((r) => r.updateAvailable).length;
  const installed = module.rovers.length;
  return (
    <button type="button" className={styles.card} onClick={() => onOpen(module.moduleId)}>
      <BracketCorners />
      <div className={styles.name}>{module.displayName}</div>
      <div className={styles.repo}>
        {module.sourceRepoUrl.replace('https://github.com/', '')} · {module.repoVisibility}
      </div>
      <div className={styles.footer}>
        <span className={styles.ver}>v<span>{module.latestVersion || '—'}</span></span>
        {behind > 0 ? (
          <Chip dot>update {module.latestVersion}</Chip>
        ) : (
          <Chip dot>on {installed} rover{installed === 1 ? '' : 's'}</Chip>
        )}
      </div>
    </button>
  );
}
