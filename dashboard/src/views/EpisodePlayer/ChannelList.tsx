// dashboard/src/views/EpisodePlayer/ChannelList.tsx
//
// Episode channel sidebar: plot channels toggle visibility, schemaless
// channels are listed with count and rate but are not plottable in v1.
import type { ChannelInfo } from '../../lib/episode/episodeSource';
import styles from './EpisodePlayerView.module.css';

type Props = {
  channels: ChannelInfo[];
  visible: string[];
  onToggle: (topic: string) => void;
};

export function ChannelList({ channels, visible, onToggle }: Props) {
  return (
    <ul className={styles.channelList}>
      {channels.filter((c) => c.kind !== 'video').map((c) => (
        <li key={c.topic} className={styles.channelRow}>
          {c.kind === 'plot' ? (
            <label className={styles.channelLabel}>
              <input
                type="checkbox"
                checked={visible.includes(c.topic)}
                onChange={() => onToggle(c.topic)}
              />
              <span>{c.topic}</span>
            </label>
          ) : (
            <span className={styles.channelMuted}>
              {c.topic}
              <span className={styles.channelNote}>not decodable</span>
            </span>
          )}
          <span className={styles.channelMeta}>
            {c.count} msgs{c.rateHz != null ? ` · ${c.rateHz.toFixed(1)} Hz` : ''}
          </span>
        </li>
      ))}
    </ul>
  );
}
