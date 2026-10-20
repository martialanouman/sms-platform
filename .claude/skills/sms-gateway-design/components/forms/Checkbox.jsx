import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function Checkbox({ label, description, checked, indeterminate = false, disabled = false, onChange, className = '', ...rest }) {
  const ref = React.useRef(null);
  React.useEffect(() => { if (ref.current) ref.current.indeterminate = indeterminate; }, [indeterminate]);
  return (
    <label className={['pl-check', disabled ? 'pl-check--disabled' : '', className].filter(Boolean).join(' ')}>
      <input ref={ref} type="checkbox" checked={checked} disabled={disabled} onChange={onChange} {...rest} />
      <span className="pl-check__box"><Icon name={indeterminate ? 'minus' : 'check'} size={11} /></span>
      {label || description ? (
        <span className="pl-check__text">
          {label}
          {description ? <span className="pl-check__desc">{description}</span> : null}
        </span>
      ) : null}
    </label>
  );
}
