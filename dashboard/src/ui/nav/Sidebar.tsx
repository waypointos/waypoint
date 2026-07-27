// dashboard/src/ui/nav/Sidebar.tsx
import React from 'react';
import { LayoutGrid, Map as MapIcon, Package, Tag, Settings, ShieldCheck, ScrollText, Users, LogOut } from 'lucide-react';
import logo from '@/assets/logo.png';
import styles from './Sidebar.module.css';
import { SidebarItem } from './SidebarItem';
import { Avatar } from '@/ui/primitives/Avatar';
import { useMe } from '@/state/mode';
import { displayInitials } from '@/state/initials';

type Props = {
  /** Collapse to icons-only (rover view). */
  collapsed?: boolean;
  /** Optional meta line shown at the very bottom (e.g. "Local · v1.2"). */
  meta?: React.ReactNode;
};

/**
 * SidebarBrand is rendered as a sibling of TopBar inside the shell grid so
 * the two share a grid row and intrinsically align their bottom borders.
 */
export function SidebarBrand({ collapsed }: { collapsed?: boolean }) {
  return (
    <div className={[styles.brand, collapsed && styles.brandCollapsed].filter(Boolean).join(' ')}>
      {collapsed ? (
        <div className={styles.brandMark}><span/><span/><span/><span/></div>
      ) : (
        <img src={logo} alt="Waypoint" />
      )}
    </div>
  );
}

export function Sidebar({ collapsed, meta }: Props) {
  const me = useMe();
  const initials = displayInitials(me);
  const showUserCard = !!me?.email;

  async function handleLogout() {
    try {
      await fetch('/auth/logout', { method: 'POST', credentials: 'include' });
    } finally {
      window.location.href = '/auth/login';
    }
  }

  return (
    <aside className={[styles.sb, collapsed && styles.collapsed].filter(Boolean).join(' ')}>
      <nav className={styles.nav}>
        <SidebarItem to="/"          end icon={<LayoutGrid  size={16} />} label="Fleet"    collapsed={collapsed} />
        <SidebarItem to="/map"       icon={<MapIcon     size={16} />} label="Map"      collapsed={collapsed} />
        <SidebarItem to="/modules"   icon={<Package     size={16} />} label="Modules"  collapsed={collapsed} />
        <SidebarItem to="/releases"  icon={<Tag         size={16} />} label="Releases" collapsed={collapsed} />
        {me?.isAdmin ? (
          <>
            <SidebarItem to="/admin"       end icon={<ShieldCheck size={16} />} label="Admin"  collapsed={collapsed} />
            <SidebarItem to="/admin/users" icon={<Users       size={16} />} label="Users"  collapsed={collapsed} />
            <SidebarItem to="/admin/audit" icon={<ScrollText  size={16} />} label="Audit"  collapsed={collapsed} />
          </>
        ) : null}
      </nav>

      <div className={styles.bottom}>
        {meta && !collapsed ? <div className={styles.meta}>{meta}</div> : null}

        <nav className={styles.bottomNav}>
          <SidebarItem to="/settings" icon={<Settings size={16} />} label="Settings" collapsed={collapsed} />
        </nav>

        {showUserCard ? (
          <div className={styles.userCard}>
            <Avatar initials={initials} />
            <div className={styles.userMeta}>
              <span className={styles.userEmail} title={me?.email}>{me?.email}</span>
              {me?.isAdmin ? <span className={styles.userRole}>Admin</span> : null}
            </div>
            <button
              type="button"
              className={styles.logout}
              onClick={handleLogout}
              aria-label="Log out"
              title="Log out"
            >
              <LogOut size={14} />
            </button>
          </div>
        ) : null}
      </div>
    </aside>
  );
}
