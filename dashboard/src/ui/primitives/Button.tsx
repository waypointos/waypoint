// dashboard/src/ui/primitives/Button.tsx
import React from 'react';
import styles from './Button.module.css';

type Variant = 'default' | 'primary' | 'danger';
type Size = 'md' | 'sm';

type Props = Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & {
  variant?: Variant;
  size?: Size;
  icon?: React.ReactNode;
  children?: React.ReactNode;
};

export function Button({ variant = 'default', size = 'md', icon, children, className, ...rest }: Props) {
  const iconOnly = !!icon && (children === undefined || children === null || children === false);
  const cls = [styles.btn, variant === 'primary' && styles.primary, variant === 'danger' && styles.danger, size === 'sm' && styles.sm, iconOnly && styles.iconOnly, className]
    .filter(Boolean)
    .join(' ');
  return (
    <button className={cls} {...rest}>
      {icon ? <span className={styles.icon}>{icon}</span> : null}
      {children}
    </button>
  );
}
