// dashboard/src/ui/nav/AlertCountBadge.tsx
//
// TopBar affordance summarising the active-alert state across the operator's
// accessible rovers. When everything is clear, renders a quiet "0 alerts"
// pill. When any severity is non-zero, renders one AlertChip per non-empty
// severity, ordered critical → warning → info.

import { Chip } from '@/ui/primitives/Chip';
import { AlertChip } from '@/ui/telemetry/AlertChip';
import type { AlertCounts } from '@/state/useAlerts';
import styles from './AlertCountBadge.module.css';

type Props = {
  counts: AlertCounts;
  /** Optional click handler — turns the badge into a button. */
  onClick?: () => void;
  /** Optional title (tooltip) override. */
  title?: string;
};

export function AlertCountBadge({ counts, onClick, title }: Props) {
  const total = counts.info + counts.warning + counts.critical;

  const content =
    total === 0 ? (
      <Chip dot>0 alerts</Chip>
    ) : (
      <>
        {counts.critical > 0 ? (
          <AlertChip severity="critical" number={counts.critical} iconOnly />
        ) : null}
        {counts.warning > 0 ? (
          <AlertChip severity="warning" number={counts.warning} iconOnly />
        ) : null}
        {counts.info > 0 ? (
          <AlertChip severity="info" number={counts.info} iconOnly />
        ) : null}
      </>
    );

  if (onClick) {
    return (
      <button
        type="button"
        className={styles.btn}
        onClick={onClick}
        title={title ?? 'Active alerts'}
        aria-label={`Active alerts: ${total}`}
      >
        {content}
      </button>
    );
  }

  return (
    <span className={styles.group} title={title ?? 'Active alerts'}>
      {content}
    </span>
  );
}
