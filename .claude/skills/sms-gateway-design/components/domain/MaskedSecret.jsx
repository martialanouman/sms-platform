import React from 'react';
import { Icon } from '../core/Icon.jsx';
import { Badge } from '../core/Badge.jsx';
import { StatusPill } from '../core/StatusPill.jsx';

export function MaskedSecret({
  kind = 'Clé API', last4, status = 'active', lastUsedAt, rotationState, liveSessions, actions, className = '',
}) {
  return (
    <div className={`pl-secret ${className}`.trim()}>
      <div className="pl-secret__head">
        <span className="pl-secret__kind">{kind}</span>
        <span style={{ marginLeft: 'auto' }}><StatusPill state={status} /></span>
      </div>
      <div className="pl-secret__value">
        <span>•••• •••• ••••</span>
        <strong style={{ fontWeight: 500 }}>{last4}</strong>
        <span style={{ marginLeft: 'auto' }}><Badge tone="neutral" appearance="outline">masqué</Badge></span>
      </div>
      <div className="pl-secret__meta">
        <div className="pl-secret__metaitem">
          <span className="pl-secret__metakey">Dernière utilisation</span>
          <span className="pl-secret__metaval">{lastUsedAt || '—'}</span>
        </div>
        <div className="pl-secret__metaitem">
          <span className="pl-secret__metakey">Rotation</span>
          <span className="pl-secret__metaval">{rotationState || 'aucune'}</span>
        </div>
        {liveSessions != null ? (
          <div className="pl-secret__metaitem">
            <span className="pl-secret__metakey">Sessions vivantes</span>
            <span className="pl-secret__metaval">{liveSessions}</span>
          </div>
        ) : null}
      </div>
      {actions ? <div className="pl-secret__actions">{actions}</div> : null}
      <div className="pl-secret__audit">
        <Icon name="ban" size={12} />
        Aucune action « révéler » : le secret n'est affiché qu'une fois, à la création.
      </div>
    </div>
  );
}
