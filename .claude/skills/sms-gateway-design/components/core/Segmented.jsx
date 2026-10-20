import React from 'react';

export function Segmented({ items = [], value, onChange, className = '', ariaLabel }) {
  return (
    <div className={`pl-seg ${className}`.trim()} role="tablist" aria-label={ariaLabel}>
      {items.map((it) => {
        const o = typeof it === 'string' ? { value: it, label: it } : it;
        return (
          <button
            type="button" key={o.value} role="tab" aria-selected={value === o.value}
            className={`pl-seg__item${value === o.value ? ' is-active' : ''}`}
            disabled={o.disabled} onClick={() => onChange && onChange(o.value)}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
