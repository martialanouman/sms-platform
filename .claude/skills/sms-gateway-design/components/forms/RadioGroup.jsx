import React from 'react';

export function RadioGroup({ name, value, options = [], row = false, disabled = false, onChange, className = '' }) {
  return (
    <div className={['pl-radiogroup', row ? 'pl-radiogroup--row' : '', className].filter(Boolean).join(' ')} role="radiogroup">
      {options.map((o) => (
        <label key={o.value} className={['pl-check', disabled || o.disabled ? 'pl-check--disabled' : ''].filter(Boolean).join(' ')}>
          <input
            type="radio" name={name} value={o.value} checked={value === o.value}
            disabled={disabled || o.disabled}
            onChange={(e) => onChange && onChange(e.target.value, e)}
          />
          <span className="pl-check__box pl-radio__dot" />
          <span className="pl-check__text">
            {o.label}
            {o.description ? <span className="pl-check__desc">{o.description}</span> : null}
          </span>
        </label>
      ))}
    </div>
  );
}
