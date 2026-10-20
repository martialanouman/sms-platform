import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function TopBar({ title, crumbs = [], badges, actions, operator, role, className = '' }) {
  const initials = operator ? operator.split(/\s+/).map((w) => w[0]).slice(0, 2).join('').toUpperCase() : null;
  return (
    <header className={`pl-topbar ${className}`.trim()}>
      {crumbs.length ? (
        <div className="pl-topbar__crumbs">
          {crumbs.map((c, i) => (
            <React.Fragment key={i}>
              {i > 0 ? <Icon name="chevron-right" size={12} /> : null}
              {c.href ? <a href={c.href}>{c.label}</a> : <span>{c.label}</span>}
            </React.Fragment>
          ))}
        </div>
      ) : null}
      {title ? <div className="pl-topbar__title">{title}{badges}</div> : null}
      <div className="pl-topbar__right">
        {actions}
        {operator ? (
          <div className="pl-topbar__op">
            <span className="pl-avatar">{initials}</span>
            <span>{operator}{role ? <span style={{ color: 'var(--text-muted)' }}> · {role}</span> : null}</span>
          </div>
        ) : null}
      </div>
    </header>
  );
}
