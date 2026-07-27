// dashboard/src/ui/nav/SidebarItem.tsx
import React from 'react';
import { NavLink, NavLinkProps } from 'react-router-dom';
import { BracketCorners } from '../primitives/BracketCorners';

type Props = Omit<NavLinkProps, 'children' | 'className'> & {
  icon: React.ReactNode;
  label: string;
  collapsed?: boolean;
};

const wrapStyle: React.CSSProperties = {
  position: 'relative',
  display: 'flex',
  alignItems: 'center',
  gap: 10,
  padding: '6px 10px',
  margin: '1px 0',
  fontSize: 12.5,
};

export function SidebarItem({ icon, label, collapsed, ...rest }: Props) {
  return (
    <NavLink
      {...rest}
      style={({ isActive }) => ({
        ...wrapStyle,
        color: isActive ? 'var(--color-fg)' : 'var(--color-fg-2)',
        textDecoration: 'none',
        justifyContent: collapsed ? 'center' : 'flex-start',
      })}
    >
      {({ isActive }) => (
        <>
          {isActive ? <BracketCorners /> : null}
          <span style={{ flexShrink: 0, color: isActive ? 'var(--color-accent)' : 'var(--color-fg-3)' }}>
            {icon}
          </span>
          {!collapsed ? <span>{label}</span> : null}
        </>
      )}
    </NavLink>
  );
}
