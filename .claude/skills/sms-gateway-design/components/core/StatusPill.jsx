import React from 'react';

const TONE = {
  up: 'up', active: 'up', delivered: 'up', connected: 'up',
  reconnecting: 'degraded', pending: 'degraded', throttled: 'degraded', expired: 'degraded',
  down: 'down', failed: 'down', suspended: 'down',
  restricted: 'restricted',
  idle: 'idle', unbound: 'idle', unknown: 'idle',
};
const LABEL = {
  up: 'up', down: 'down', reconnecting: 'reconnecting', active: 'active', suspended: 'suspended',
  delivered: 'delivered', failed: 'failed', pending: 'pending', throttled: 'throttled',
  expired: 'expired', restricted: 'restricted', unbound: 'unbound', unknown: 'unknown', connected: 'connected',
};
const BREAKER = { closed: 'closed', open: 'open', half_open: 'half_open' };

/**
 * Charte §06 — link_status se rend en POINT + libellé mono, breaker_state en
 * PILULE teintée. Jamais l'inverse, jamais fusionnés.
 */
export function StatusPill({ state, label, meta, live = false, note, className = '' }) {
  if (BREAKER[state]) {
    return (
      <span className={className || undefined}>
        <span className={`pl-breaker pl-breaker--${state}`}>{label || BREAKER[state]}</span>
        {note ? <span className="pl-breaker__note">{note}</span> : null}
      </span>
    );
  }
  const tone = TONE[state] || 'idle';
  const cls = ['pl-status', `pl-status--${tone}`, live ? 'pl-status--live' : '', className].filter(Boolean).join(' ');
  return (
    <span className={cls}>
      <span className="pl-status__dot" />
      <span className="pl-status__label">{label || LABEL[state] || state}</span>
      {meta ? <span className="pl-status__meta">{meta}</span> : null}
    </span>
  );
}
