import React from 'react';
import { Icon } from '../core/Icon.jsx';

const ICON = { info: 'info', success: 'check', warning: 'warning', danger: 'bang', neutral: 'info' };

export function Banner({ tone = 'info', title, children, icon, actions, className = '' }) {
  return (
    <div className={['pl-banner', `pl-banner--${tone}`, className].filter(Boolean).join(' ')} role={tone === 'danger' ? 'alert' : undefined}>
      <span className="pl-banner__icon"><Icon name={icon || ICON[tone]} size={15} /></span>
      <div className="pl-banner__body">
        {title ? <span className="pl-banner__title">{title}</span> : null}
        {children ? <span className="pl-banner__text">{children}</span> : null}
      </div>
      {actions ? <div className="pl-banner__actions">{actions}</div> : null}
    </div>
  );
}
