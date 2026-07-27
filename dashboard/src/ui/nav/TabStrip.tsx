// Full-width tab row that sits under the TopBar. Brutalist treatment:
// uppercase mono labels, active tab gets a bottom underline in --color-accent,
// no background fills. Active state derives from the URL via NavLink.
import { NavLink } from 'react-router-dom';
import styles from './TabStrip.module.css';

export type TabStripEntry = {
  id: string;
  label: string;
  to: string;
};

type Props = {
  tabs: TabStripEntry[];
};

export function TabStrip({ tabs }: Props) {
  return (
    <nav className={styles.strip} role="tablist" aria-label="Rover view tabs">
      {tabs.map((t) => (
        <NavLink
          key={t.id}
          to={t.to}
          end
          className={({ isActive }) =>
            isActive ? `${styles.tab} ${styles.active}` : styles.tab
          }
        >
          {({ isActive }) => (
            <span
              role="tab"
              aria-selected={isActive}
              tabIndex={isActive ? 0 : -1}
              className={styles.label}
            >
              {t.label}
            </span>
          )}
        </NavLink>
      ))}
    </nav>
  );
}
