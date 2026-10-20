import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function Tabs({ tabs = [], activeId, onChange, className = '' }) {
  return (
    <div className={`pl-tabs ${className}`.trim()} role="tablist">
      {tabs.map((t) => (
        <button
          type="button" key={t.id} role="tab" aria-selected={activeId === t.id}
          className={`pl-tab${activeId === t.id ? ' is-active' : ''}`}
          disabled={t.disabled} onClick={() => onChange && onChange(t.id)}
        >
          {t.icon ? <Icon name={t.icon} size={13} /> : null}
          {t.label}
          {t.count != null ? <span className="pl-tab__count">{t.count}</span> : null}
        </button>
      ))}
    </div>
  );
}
