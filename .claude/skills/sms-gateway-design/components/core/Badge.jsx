import React from 'react';

export function Badge({ tone = 'neutral', appearance = 'soft', children, className = '' }) {
  const cls = ['pl-badge', `pl-badge--${tone}`, appearance !== 'soft' ? `pl-badge--${appearance}` : '', className].filter(Boolean).join(' ');
  return <span className={cls}>{children}</span>;
}
