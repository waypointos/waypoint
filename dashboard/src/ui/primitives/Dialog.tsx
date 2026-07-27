// dashboard/src/ui/primitives/Dialog.tsx
//
// Lightweight modal overlay. Closes on Escape or backdrop click.
// Renders via createPortal so it escapes stacking contexts.
import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { BracketCorners } from './BracketCorners';
import styles from './Dialog.module.css';

type Props = {
  open: boolean;
  onClose: () => void;
  title: string;
  /** `wide` gives a large surface for media (e.g. the 3D map). */
  size?: 'default' | 'wide';
  /** Chromeless: no header/border/padding, just the content + a floating close.
      For edge-to-edge media like a full-bleed map. `title` becomes the aria label. */
  bare?: boolean;
  children?: React.ReactNode;
};

export function Dialog({ open, onClose, title, size = 'default', bare = false, children }: Props) {
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [open, onClose]);

  if (!open) return null;

  const surfaceClass = [
    styles.surface,
    size === 'wide' && styles.surfaceWide,
    bare && styles.surfaceBare,
  ].filter(Boolean).join(' ');

  return createPortal(
    <div className={styles.backdrop} onMouseDown={onClose}>
      <div
        className={surfaceClass}
        onMouseDown={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        {...(bare ? { 'aria-label': title } : { 'aria-labelledby': 'dialog-title' })}
      >
        {bare ? (
          <button className={styles.closeFloat} onClick={onClose} aria-label="Close">
            <X size={16} />
          </button>
        ) : (
          <header className={styles.header}>
            <div className={styles.titleWrap}>
              <BracketCorners full size={6} />
              <span id="dialog-title" className={styles.title}>{title}</span>
            </div>
            <button className={styles.close} onClick={onClose} aria-label="Close">
              <X size={14} />
            </button>
          </header>
        )}
        <div className={styles.body}>{children}</div>
      </div>
    </div>,
    document.body,
  );
}
