import React from 'react';

export function Switch({ label, checked, disabled = false, onChange, className = '', ...rest }) {
  return (
    <label className={['pl-switch', disabled ? 'pl-switch--disabled' : '', className].filter(Boolean).join(' ')}>
      <input type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={onChange} {...rest} />
      <span className="pl-switch__track"><span className="pl-switch__knob" /></span>
      {label ? <span>{label}</span> : null}
    </label>
  );
}
