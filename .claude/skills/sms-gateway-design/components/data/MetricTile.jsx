import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function MetricTile({ label, value, unit, tone = 'default', delta, deltaDirection, live = false, footer, size = 'md', className = '' }) {
  const cls = ['pl-metric', tone !== 'default' ? `pl-metric--${tone}` : '', size === 'sm' ? 'pl-metric--sm' : '', className].filter(Boolean).join(' ');
  return (
    <div className={cls}>
      <div className="pl-metric__inner">
        <div className="pl-metric__label">
          {label}
          {live ? <span className="pl-status__dot pl-status--live" style={{ background: 'var(--teal-500)', width: 6, height: 6 }} /> : null}
        </div>
        <div className="pl-metric__value">{value}{unit ? <span className="pl-metric__unit">{unit}</span> : null}</div>
        {delta || footer ? (
          <div className="pl-metric__foot">
            {delta ? (
              <span className={deltaDirection === 'down' ? 'pl-metric__delta--down' : 'pl-metric__delta--up'}>
                <Icon name={deltaDirection === 'down' ? 'arrow-down' : 'arrow-up'} size={11} />{delta}
              </span>
            ) : null}
            {footer}
          </div>
        ) : null}
      </div>
    </div>
  );
}
