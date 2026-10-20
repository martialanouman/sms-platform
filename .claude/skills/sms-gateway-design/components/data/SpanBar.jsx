import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function SpanBar({ name, depth = 0, startPct = 0, widthPct = 10, duration, state = 'ok', icon, className = '' }) {
  const cls = ['pl-span', `pl-span--${state}`, className].filter(Boolean).join(' ');
  return (
    <div className={cls}>
      <div className="pl-span__name" style={{ paddingLeft: depth * 12 }}>
        {depth > 0 ? <span className="pl-span__depth">└</span> : null}
        {icon ? <Icon name={icon} size={12} /> : null}
        {name}
      </div>
      <div className="pl-span__track">
        <div className="pl-span__bar" style={{ left: `${startPct}%`, width: `${Math.max(widthPct, 0.6)}%` }} />
      </div>
      <div className="pl-span__dur">{duration}</div>
    </div>
  );
}
