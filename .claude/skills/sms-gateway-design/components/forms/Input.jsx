import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function Input({ icon, mono = false, size = 'md', invalid = false, className = '', ...rest }) {
  const cls = ['pl-input', mono ? 'pl-input--mono' : '', size === 'sm' ? 'pl-input--sm' : '', invalid ? 'pl-input--invalid' : '', className].filter(Boolean).join(' ');
  const input = <input className={cls} aria-invalid={invalid || undefined} {...rest} />;
  if (!icon) return input;
  return (
    <span className="pl-inputwrap pl-inputwrap--icon">
      <span className="pl-inputwrap__icon"><Icon name={icon} size={13} /></span>
      {input}
    </span>
  );
}
