import React from 'react';
import { Badge } from '../core/Badge.jsx';

const fr = (n) => (typeof n === 'number' ? n.toLocaleString('fr-FR').replace(/\u202f|\u00a0/g, ' ') : n);

export function BalanceCard({
  direction = 'mt', value, unit = 'crédits SMS', note, meterPct, scope, accountLabel, actions, className = '',
}) {
  const isMt = direction === 'mt';
  return (
    <div className={['pl-balance', `pl-balance--${direction}`, className].filter(Boolean).join(' ')}>
      <div className="pl-balance__head">
        <Badge tone={direction}>{isMt ? 'MT' : 'MO'}</Badge>
        <span className="pl-balance__label">{isMt ? 'Solde MT' : 'Compteur MO'}</span>
        {accountLabel ? <span className="pl-card__sub" style={{ marginLeft: 'auto' }}>{accountLabel}</span> : null}
      </div>
      <div className="pl-balance__value">{fr(value)}<span className="pl-balance__unit">{unit}</span></div>
      {meterPct != null ? (
        <div className="pl-balance__meter"><div className="pl-balance__meterfill" style={{ width: `${Math.min(Math.max(meterPct, 0), 100)}%` }} /></div>
      ) : null}
      {note ? <p className="pl-balance__note" style={{ margin: 0 }}>{note}</p> : null}
      {scope || actions ? (
        <div className="pl-balance__foot">
          {scope ? <Badge tone="neutral" appearance="outline">{scope}</Badge> : null}
          {actions ? <span style={{ marginLeft: 'auto', display: 'flex', gap: 'var(--sp-4)' }}>{actions}</span> : null}
        </div>
      ) : null}
    </div>
  );
}
