import React from 'react';
import { Icon } from './Icon.jsx';

export function IconButton({ icon, label, variant = 'ghost', size = 'md', disabled = false, onClick, className = '', ...rest }) {
  const cls = ['pl-iconbtn', variant !== 'ghost' ? `pl-iconbtn--${variant}` : '', size === 'sm' ? 'pl-iconbtn--sm' : '', className].filter(Boolean).join(' ');
  return (
    <button type="button" className={cls} title={label} aria-label={label} disabled={disabled} onClick={onClick} {...rest}>
      <Icon name={icon} size={size === 'sm' ? 13 : 15} />
    </button>
  );
}
