// dashboard/src/ui/primitives/Drawer.tsx
//
// Right-side slide-in panel. Portal-rendered, scrim + Escape close,
// body scroll-locked while open. The rail and footer are optional slots
// for fixed left/bottom chrome; children go in the scrolling middle.
import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import styles from './Drawer.module.css';

type Props = {
  open: boolean;
  onClose: () => void;
  rail?: React.ReactNode;
  footer?: React.ReactNode;
  children?: React.ReactNode;
  'aria-label'?: string;
};

export function Drawer({ open, onClose, rail, footer, children, 'aria-label': ariaLabel }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    // Scroll-lock assumes one overlay at a time. Stacking with Dialog would need a ref-counter.
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    window.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener('keydown', onKey);
    };
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <>
      <div
        data-testid="drawer-scrim"
        className={styles.scrim}
        onMouseDown={onClose}
      />
      <aside
        data-testid="drawer-panel"
        className={styles.panel}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        onMouseDown={(e) => e.stopPropagation()}
      >
        {rail ? <div className={styles.rail}>{rail}</div> : null}
        <div className={styles.column}>
          <div className={styles.body}>{children}</div>
          {footer ? <div className={styles.footer}>{footer}</div> : null}
        </div>
      </aside>
    </>,
    document.body,
  );
}
