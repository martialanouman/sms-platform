import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function Toast({ severity = 'info', title, children, source, onClose, className = '' }) {
  return (
    <div className={['pl-toast', `pl-toast--${severity}`, className].filter(Boolean).join(' ')} role="status">
      <span className="pl-toast__dot" />
      <div className="pl-toast__body">
        {title ? <span className="pl-toast__title">{title}</span> : null}
        {children ? <span className="pl-toast__text">{children}</span> : null}
        {source ? <span className="pl-toast__text">source · {source}</span> : null}
      </div>
      {onClose ? (
        <button type="button" className="pl-toast__close" onClick={onClose} aria-label="Fermer">
          <Icon name="x" size={13} />
        </button>
      ) : null}
    </div>
  );
}

export function ToastStack({ children, className = '' }) {
  return <div className={`pl-toaststack ${className}`.trim()}>{children}</div>;
}
