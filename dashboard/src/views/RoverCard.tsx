// dashboard/src/views/RoverCard.tsx
import React from 'react';
import { Link } from 'react-router-dom';
import { Battery, Signal, Gamepad2 } from 'lucide-react';
import roverImg from '@/assets/rover.png';
import styles from './RoverCard.module.css';
import { BracketCorners } from '@/ui/primitives/BracketCorners';

export type RoverSummary = {
  id: string;
  nick: string;
  status: 'online' | 'caution' | 'offline';
  mode: string | null;
  busVoltage: number | null;
  link: string | null;
  version: string | null;
};

type Props = { rover: RoverSummary };

export function RoverCard({ rover }: Props) {
  const off = rover.status === 'offline';
  return (
    <Link to={`/rover/${rover.id}`} style={{ textDecoration: 'none', color: 'inherit' }}>
      <article className={[styles.card, off && styles.off].filter(Boolean).join(' ')}>
        <div className={styles.vis}>
          <span className={styles.pin}>{rover.status === 'caution' ? 'Online · safe' : rover.status}</span>
          <span className={styles.ver}>{rover.version ?? '—'}</span>
          <div className={styles.rover} style={{ backgroundImage: `url(${roverImg})` }} />
        </div>
        <div className={styles.body}>
          <div className={styles.name}>{rover.id}</div>
          <div className={styles.nick}>{rover.nick}</div>
          <Row icon={<Gamepad2 size={13} />} label="Mode"        value={rover.mode ?? 'N/A'} />
          <Row icon={<Battery  size={13} />} label="Bus voltage" value={rover.busVoltage != null ? `${rover.busVoltage.toFixed(1)} V` : 'N/A'} />
          <Row icon={<Signal   size={13} />} label="Link"        value={rover.link ?? 'N/A'} />
        </div>
        <BracketCorners />
      </article>
    </Link>
  );
}

function Row({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className={styles.row}>
      <span className={styles.l}>{icon}{label}</span>
      <span className={styles.v}>{value}</span>
    </div>
  );
}
