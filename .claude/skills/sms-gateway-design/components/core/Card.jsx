import React from 'react';

export function Card({ title, subtitle, actions, footer, flush = false, flat = false, sunken = false, children, className = '', ...rest }) {
  const cls = ['pl-card', flat ? 'pl-card--flat' : '', sunken ? 'pl-card--sunken' : '', className].filter(Boolean).join(' ');
  return (
    <section className={cls} {...rest}>
      {title || actions ? (
        <header className="pl-card__head">
          <div>
            {title ? <h3 className="pl-card__title">{title}</h3> : null}
            {subtitle ? <div className="pl-card__sub">{subtitle}</div> : null}
          </div>
          {actions ? <div className="pl-card__actions">{actions}</div> : null}
        </header>
      ) : null}
      <div className={`pl-card__body${flush ? ' pl-card__body--flush' : ''}`}>{children}</div>
      {footer ? <div className="pl-card__foot">{footer}</div> : null}
    </section>
  );
}
