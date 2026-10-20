import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function ErrorState({ title = 'Impossible de joindre l\'API Admin', children, request, action, className = '' }) {
  return (
    <div
      className={className || undefined}
      style={{
        display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--sp-5)',
        padding: 'var(--sp-9)', textAlign: 'center',
        background: 'var(--tint-red)', border: '1px solid color-mix(in srgb, var(--red-500) 30%, transparent)',
        borderRadius: 'var(--radius-card)',
      }}
      role="alert"
    >
      <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 32, height: 32, borderRadius: 'var(--radius-control)', background: 'color-mix(in srgb, var(--red-500) 18%, transparent)', color: 'var(--red-500)' }}>
        <Icon name="bang" size={16} />
      </span>
      <span style={{ font: 'var(--text-card-title)', color: 'var(--text-primary)' }}>{title}</span>
      {children ? <span style={{ fontSize: 'var(--fs-12)', color: 'var(--text-muted)', maxWidth: '44ch', lineHeight: 'var(--lh-normal)' }}>{children}</span> : null}
      {request ? <span style={{ font: 'var(--text-data-sm)', color: 'var(--text-faint)' }}>{request}</span> : null}
      {action}
    </div>
  );
}
