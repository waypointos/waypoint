// dashboard/src/ui/telemetry/EstopButton.tsx
//
// The E-stop control. When the rover is estopped it flips to a Clear action
// (rpc.recover). Engage stays reachable whenever the rover is online.
import type { Mode as DisplayMode } from './ModeIndicator';
import styles from './EstopButton.module.css';

type Props = {
  mode: DisplayMode;
  offline: boolean;
  clearing?: boolean;
  onEngage: () => void;
  onClear: () => void;
};

export function EstopButton({ mode, offline, clearing = false, onEngage, onClear }: Props) {
  if (mode === 'estop') {
    return (
      <button type="button" className={styles.clear} data-estop-clear disabled={offline || clearing} onClick={onClear}>
        {clearing ? 'Clearing…' : 'Clear E-Stop'}
      </button>
    );
  }
  return (
    <button type="button" className={styles.estop} data-estop disabled={offline} onClick={onEngage}>
      Emergency Stop
    </button>
  );
}
