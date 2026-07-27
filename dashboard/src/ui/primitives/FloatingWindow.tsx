// dashboard/src/ui/primitives/FloatingWindow.tsx
//
// Reusable draggable + resizable floating window. Title bar uses the mono-caps
// bracket vocabulary; a bottom-right handle resizes. Move/resize live in
// useDragResize; pure clamp geometry in ./floatingWindowGeometry.
import React, { useState } from 'react';
import { X, Minus, Plus } from 'lucide-react';
import { BracketCorners } from './BracketCorners';
import { useDragResize } from './useDragResize';
import type { Point, Size } from './floatingWindowGeometry';
import styles from './FloatingWindow.module.css';

type Props = {
  title: string;
  children: React.ReactNode;
  initialPosition?: Point;
  initialSize?: Size;
  minSize?: Size;
  onClose?: () => void;
};

export function FloatingWindow({
  title, children, onClose, initialPosition, initialSize, minSize,
}: Props) {
  const { pos, size, startMove, startResize } = useDragResize({ initialPosition, initialSize, minSize });
  const [collapsed, setCollapsed] = useState(false);

  return (
    <section
      data-testid="floating-window"
      className={styles.window}
      style={{ left: pos.x, top: pos.y, width: size.width, height: collapsed ? undefined : size.height }}
    >
      <BracketCorners />
      <header
        data-testid="floating-window-titlebar"
        className={styles.titlebar}
        onPointerDown={startMove}
      >
        <span className={styles.title}>
          <span className={styles.bracket}>[</span>{title}<span className={styles.bracket}>]</span>
        </span>
        <div className={styles.controls}>
          <button
            type="button"
            className={styles.ctl}
            aria-label={collapsed ? 'expand' : 'collapse'}
            onClick={() => setCollapsed((c) => !c)}
          >
            {collapsed ? <Plus size={12} /> : <Minus size={12} />}
          </button>
          {onClose && (
            <button type="button" className={styles.ctl} aria-label="close" onClick={onClose}>
              <X size={12} />
            </button>
          )}
        </div>
      </header>
      {!collapsed && <div className={styles.body}>{children}</div>}
      {!collapsed && (
        <span
          className={styles.resize}
          aria-label="resize"
          role="separator"
          onPointerDown={startResize}
        />
      )}
    </section>
  );
}
