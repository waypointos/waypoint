// dashboard/src/ui/primitives/WizardModal.tsx
//
// Centered wizard modal with a labeled vertical stepper sidebar (macOS-installer
// style). Portal-rendered, scrim + Escape close, body scroll-locked while open.
// The title bar, footer, and stepper are optional slots; children go in the
// scrolling body column. Scroll-lock assumes one overlay at a time.
import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import styles from './WizardModal.module.css';

export type WizardStep = {
  number: string;
  label: string;
  state: 'done' | 'current' | 'upcoming';
};

type Props = {
  open: boolean;
  onClose: () => void;
  title?: string;
  steps: WizardStep[];
  footer?: React.ReactNode;
  children?: React.ReactNode;
  'aria-label'?: string;
};

export function WizardModal({
  open,
  onClose,
  title,
  steps,
  footer,
  children,
  'aria-label': ariaLabel,
}: Props) {
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
        data-testid="wizard-modal-scrim"
        className={styles.scrim}
        onMouseDown={onClose}
      />
      <div
        data-testid="wizard-modal-panel"
        className={styles.panel}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        onMouseDown={(e) => e.stopPropagation()}
      >
        {title ? (
          <div className={styles.titleBar}>
            <span className={styles.titleText}>
              <span className={styles.titleBk}>[</span>
              {title}
              <span className={styles.titleBk}>]</span>
            </span>
            <button
              type="button"
              className={styles.closeBtn}
              aria-label="Close"
              onClick={onClose}
            >
              ×
            </button>
          </div>
        ) : null}

        <div className={styles.content}>
          <aside className={styles.sidebar}>
            <ol className={styles.stepper}>
              {steps.map((step) => (
                <li
                  key={step.number}
                  data-step-number={step.number}
                  className={`${styles.step} ${styles['step-' + step.state]}`}
                  aria-current={step.state === 'current' ? 'step' : undefined}
                >
                  <span className={styles.stepNum}>{step.number}</span>
                  <span className={styles.stepLabel}>{step.label}</span>
                </li>
              ))}
            </ol>
          </aside>

          <div className={styles.body}>{children}</div>
        </div>

        {footer ? <div className={styles.footer}>{footer}</div> : null}
      </div>
    </>,
    document.body,
  );
}
