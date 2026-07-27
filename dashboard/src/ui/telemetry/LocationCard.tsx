// Overview LOCATION card: compact top-down 2D map with a jump-out to the 3D view.
// With no GPS fix it shows Paris and no rover marker (first-class N/A).
import { useState } from 'react';
import { ExternalLink } from 'lucide-react';
import { Panel } from '@/ui/primitives/Panel';
import { Button } from '@/ui/primitives/Button';
import { Dialog } from '@/ui/primitives/Dialog';
import { LocationMap, type LngLat, type RoverMarker } from './LocationMap';
import styles from './LocationCard.module.css';

type Props = {
  roverId: string;
  nick?: string;
  /** Current fix, or null when there's no GPS. */
  position?: LngLat | null;
};

export function LocationCard({ roverId, nick, position = null }: Props) {
  const [open, setOpen] = useState(false);

  const markers: RoverMarker[] = position ? [{ id: roverId, nick: nick ?? roverId, position }] : [];
  const coords = position
    ? `${position.lat.toFixed(4)}, ${position.lng.toFixed(4)}`
    : 'NO FIX · PARIS';

  return (
    <Panel
      title="LOCATION"
      note={position ? 'live · gps' : 'no fix · paris'}
      action={<Button size="sm" icon={<ExternalLink size={14} />} aria-label="Open 3D map" onClick={() => setOpen(true)} />}
    >
      <div className={styles.frame}>
        <LocationMap markers={markers} controls={false} />
        <div className={styles.hud}>{coords}</div>
      </div>

      <Dialog open={open} onClose={() => setOpen(false)} title="MAP · 3D" size="wide" bare>
        <div className={styles.dialogMap}>
          <LocationMap pitched markers={markers} />
        </div>
      </Dialog>
    </Panel>
  );
}
