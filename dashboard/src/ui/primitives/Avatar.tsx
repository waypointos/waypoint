// dashboard/src/ui/primitives/Avatar.tsx
import styles from './Avatar.module.css';

type Props = {
  /** 2-letter initials (BE, AB). Caller decides what to show. */
  initials: string;
};

export function Avatar({ initials }: Props) {
  return <span className={styles.avatar}>{initials.slice(0, 2).toUpperCase()}</span>;
}
