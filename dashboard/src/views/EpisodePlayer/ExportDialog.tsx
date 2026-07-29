// dashboard/src/views/EpisodePlayer/ExportDialog.tsx
//
// Client-side CSV export: pick series and range, build the wide CSV, hand it
// to the browser as a Blob download. The rover never does export work.
import { useState } from 'react';
import { Dialog } from '@/ui/primitives/Dialog';
import { Button } from '@/ui/primitives/Button';
import { buildCsv } from '../../lib/episode/csv';
import type { Sample } from '../../lib/episode/decoders';
import styles from './EpisodePlayerView.module.css';

function blobDownload(filename: string, csv: string): void {
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

type Props = {
  open: boolean;
  onClose: () => void;
  series: Map<string, Sample[]>;
  visibleIds: string[];
  range: { startNs: bigint; endNs: bigint };
  selection: { startNs: bigint; endNs: bigint } | null;
  episodeId: string;
  /** Injected so tests can intercept the Blob/anchor step. */
  download?: (filename: string, csv: string) => void;
};

export function ExportDialog({
  open, onClose, series, visibleIds, range, selection, episodeId,
  download = blobDownload,
}: Props) {
  const [checked, setChecked] = useState<string[]>(visibleIds);
  const win = selection ?? range;

  function toggle(id: string) {
    setChecked((c) => (c.includes(id) ? c.filter((x) => x !== id) : [...c, id]));
  }

  function doExport() {
    const ids = [...series.keys()].filter((id) => checked.includes(id));
    download(`${episodeId}.csv`, buildCsv(series, ids, win.startNs, win.endNs));
    onClose();
  }

  return (
    <Dialog open={open} onClose={onClose} title="Export CSV">
      <ul className={styles.exportList}>
        {[...series.keys()].map((id) => (
          <li key={id}>
            <label className={styles.channelLabel}>
              <input
                type="checkbox"
                aria-label={id}
                checked={checked.includes(id)}
                onChange={() => toggle(id)}
              />
              <span>{id}</span>
            </label>
          </li>
        ))}
      </ul>
      <p className={styles.exportRange}>
        {selection ? 'Exporting the selected time range.' : 'Exporting the full episode.'}
      </p>
      <div className={styles.confirmActions}>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" onClick={doExport}>Export</Button>
      </div>
    </Dialog>
  );
}
