import type { CSSProperties } from 'react';
import type { Tokens } from './types';

// The panel mounts inside the dashboard document, so the host's --color-*
// custom properties are normally in scope; ctx.tokens seeds --sv-* locals
// so the bundle also renders under any future host. CSS reads
// var(--sv-*, var(--color-*)) and never a raw hex.
export function tokenVars(t?: Tokens): CSSProperties {
  const c = t?.colors ?? {};
  const f = t?.fonts ?? {};
  const v: Record<string, string> = {};
  const set = (key: string, val?: string) => {
    if (val) v[key] = val;
  };
  set('--sv-bg', c.bg);
  set('--sv-surface', c.surface);
  set('--sv-surface-2', c.surface2);
  set('--sv-raise', c.raise);
  set('--sv-border', c.border);
  set('--sv-fg', c.fg);
  set('--sv-fg-2', c.fg2);
  set('--sv-fg-3', c.fg3);
  set('--sv-fg-4', c.fg4);
  set('--sv-accent', c.accent);
  set('--sv-accent-tint', c.accentTint);
  set('--sv-caution', c.caution);
  set('--sv-ok', c.ok);
  set('--sv-fault', c.fault);
  set('--sv-mono', f.mono);
  set('--sv-sans', f.sans);
  return v as CSSProperties;
}
