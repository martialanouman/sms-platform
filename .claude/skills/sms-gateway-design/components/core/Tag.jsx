import React from 'react';
import { Icon } from './Icon.jsx';

export function Tag({ children, mono = false, onRemove, className = '' }) {
  const cls = ['pl-tag', mono ? 'pl-tag--mono' : '', onRemove ? '' : 'pl-tag--static', className].filter(Boolean).join(' ');
  return (
    <span className={cls}>
      {children}
      {onRemove ? (
        <button type="button" className="pl-tag__x" onClick={onRemove} aria-label="Retirer">
          <Icon name="x" size={11} />
        </button>
      ) : null}
    </span>
  );
}
