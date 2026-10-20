import React from 'react';
import { Icon } from './Icon.jsx';

export function Button({
  variant = 'secondary', size = 'md', icon, iconAfter, loading = false,
  disabled = false, fullWidth = false, type = 'button', onClick, children, className = '', ...rest
}) {
  const cls = [
    'pl-btn', `pl-btn--${variant}`,
    size !== 'md' ? `pl-btn--${size}` : '',
    fullWidth ? 'pl-btn--block' : '', className,
  ].filter(Boolean).join(' ');
  const gl = size === 'lg' ? 16 : 14;
  return (
    <button type={type} className={cls} disabled={disabled || loading} onClick={onClick} {...rest}>
      {loading ? <span className="pl-btn__spin" /> : icon ? <Icon name={icon} size={gl} /> : null}
      {children}
      {iconAfter ? <Icon name={iconAfter} size={gl} /> : null}
    </button>
  );
}
