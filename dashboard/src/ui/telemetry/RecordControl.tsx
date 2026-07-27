// dashboard/src/ui/telemetry/RecordControl.tsx
//
// Teleop-strip episode recorder control. Idle: a REC button that prompts for a
// task label. Recording: elapsed time + size and a Stop button that prompts for
// the success/failure outcome. Optimistic: state follows the rover's
// event.recorder stream (see useRoverData), never a request/reply.
import { useState } from 'react';
import { Button } from '../primitives/Button';
import { Dialog } from '../primitives/Dialog';
import { startRecord, stopRecord } from '../../state/roverCommands';
import type { RecorderStatus } from '../../views/RoverView/useRoverData';
import styles from './RecordControl.module.css';

function fmtElapsed(s: number): string {
  const m = Math.floor(s / 60);
  const r = Math.floor(s % 60);
  return `${m}:${String(r).padStart(2, '0')}`;
}

export function RecordControl({ roverId, recorder }: { roverId: string; recorder: RecorderStatus | null }) {
  const [labelOpen, setLabelOpen] = useState(false);
  const [stopOpen, setStopOpen] = useState(false);
  const [label, setLabel] = useState('');
  const recording = recorder?.state === 'recording';

  if (recording) {
    return (
      <div className={styles.live}>
        <span className={styles.dot} aria-hidden />
        <span className={styles.elapsed}>{fmtElapsed(recorder.elapsedS)}</span>
        <span className={styles.bytes}>{(recorder.bytes / (1024 * 1024)).toFixed(1)} MB</span>
        <Button size="sm" onClick={() => setStopOpen(true)}>Stop</Button>
        <Dialog open={stopOpen} onClose={() => setStopOpen(false)} title="Episode outcome">
          <div className={styles.outcome}>
            <Button variant="primary" onClick={() => { stopRecord(roverId, true, ''); setStopOpen(false); }}>Success</Button>
            <Button variant="danger" onClick={() => { stopRecord(roverId, false, ''); setStopOpen(false); }}>Failure</Button>
          </div>
        </Dialog>
      </div>
    );
  }

  const blocked = recorder !== null && !recorder.canStart;
  return (
    <>
      <Button
        size="sm"
        disabled={blocked}
        title={blocked ? recorder.reason : 'record an episode'}
        onClick={() => setLabelOpen(true)}
      >
        ● REC
      </Button>
      <Dialog open={labelOpen} onClose={() => setLabelOpen(false)} title="Record episode">
        <label className={styles.field}>
          Task label
          <input value={label} onChange={(e) => setLabel(e.target.value)} autoFocus />
        </label>
        <Button
          variant="primary"
          disabled={!label.trim()}
          onClick={() => { startRecord(roverId, label.trim()); setLabelOpen(false); setLabel(''); }}
        >
          Start
        </Button>
      </Dialog>
    </>
  );
}
