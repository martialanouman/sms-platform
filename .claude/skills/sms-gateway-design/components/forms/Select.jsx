import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function Select({ options = [], size = 'md', placeholder, className = '', ...rest }) {
  const cls = ['pl-select', size === 'sm' ? 'pl-select--sm' : '', className].filter(Boolean).join(' ');
  return (
    <span className="pl-selectwrap">
      <select className={cls} {...rest}>
        {placeholder ? <option value="">{placeholder}</option> : null}
        {options.map((o) => {
          const opt = typeof o === 'string' ? { value: o, label: o } : o;
          return <option key={opt.value} value={opt.value} disabled={opt.disabled}>{opt.label}</option>;
        })}
      </select>
      <span className="pl-selectwrap__caret"><Icon name="chevron-down" size={13} /></span>
    </span>
  );
}
