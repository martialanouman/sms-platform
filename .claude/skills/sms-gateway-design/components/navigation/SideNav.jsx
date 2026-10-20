import React from 'react';
import { Icon } from '../core/Icon.jsx';
import { Badge } from '../core/Badge.jsx';

export function SideNav({ monogram = 'SG', wordmark = 'SMS Gateway', env, groups = [], activeId, onNavigate, className = '' }) {
  return (
    <nav className={`pl-nav ${className}`.trim()} aria-label="Navigation principale">
      <div className="pl-nav__brand">
        <span className="pl-nav__logo" aria-hidden="true">{monogram}</span>
        <span className="pl-nav__wordmark">{wordmark}</span>
        {env ? <span className="pl-nav__env"><Badge tone="accent">{env}</Badge></span> : null}
      </div>
      {groups.map((g) => (
        <div className="pl-nav__group" key={g.label}>
          <div className="pl-nav__grouplabel">{g.label}</div>
          {g.items.map((it) => (
            <button
              type="button" key={it.id}
              className={`pl-nav__item${activeId === it.id ? ' is-active' : ''}`}
              onClick={() => onNavigate && onNavigate(it.id)}
            >
              {it.icon ? <Icon name={it.icon} size={13} /> : null}
              {it.label}
              {it.count != null ? <span className="pl-nav__count">{it.count}</span> : null}
            </button>
          ))}
        </div>
      ))}
    </nav>
  );
}
