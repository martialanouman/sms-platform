import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function EmptyState({ icon = 'diamond', title, children, action, inline = false, bare = false, className = '' }) {
  return (
    <div className={['pl-empty', inline ? 'pl-empty--inline' : '', bare ? 'pl-empty--bare' : '', className].filter(Boolean).join(' ')}>
      <span className="pl-empty__icon"><Icon name={icon} size={16} /></span>
      {title ? <div className="pl-empty__title">{title}</div> : null}
      {children ? <div className="pl-empty__text">{children}</div> : null}
      {action}
    </div>
  );
}
