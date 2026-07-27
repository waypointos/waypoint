// dashboard/src/ui/nav/Crumbs.tsx
import React from 'react';
import { Link } from 'react-router-dom';

type Crumb = { label: string; to?: string };

type Props = { items: Crumb[] };

const sep: React.CSSProperties = { color: 'var(--color-fg-4)' };
const styleNow: React.CSSProperties = { color: 'var(--color-fg)', fontWeight: 500 };
const styleMuted: React.CSSProperties = { color: 'var(--color-fg-3)' };

export function Crumbs({ items }: Props) {
  return (
    <nav style={{
      display: 'inline-flex', alignItems: 'center', gap: 8,
      fontSize: 13, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em',
    }}>
      {items.map((c, i) => (
        <React.Fragment key={i}>
          {i > 0 && <span style={sep}>/</span>}
          {c.to ? (
            <Link to={c.to} style={styleMuted}>{c.label}</Link>
          ) : (
            <span style={i === items.length - 1 ? styleNow : styleMuted}>{c.label}</span>
          )}
        </React.Fragment>
      ))}
    </nav>
  );
}
