import React from 'react';
import { IconButton } from '../core/IconButton.jsx';

export function Modal({ open = true, title, icon, wide = false, onClose, footer, children, className = '' }) {
  React.useEffect(() => {
    if (!open || !onClose) return;
    const onKey = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="pl-scrim" onClick={(e) => { if (e.target === e.currentTarget && onClose) onClose(); }}>
      <div className={['pl-modal', wide ? 'pl-modal--wide' : '', className].filter(Boolean).join(' ')} role="dialog" aria-modal="true" aria-label={typeof title === 'string' ? title : undefined}>
        <header className="pl-modal__head">
          {icon}
          <h2 className="pl-modal__title">{title}</h2>
          {onClose ? <span style={{ marginLeft: 'auto' }}><IconButton icon="x" label="Fermer" onClick={onClose} /></span> : null}
        </header>
        <div className="pl-modal__body">{children}</div>
        {footer ? <div className="pl-modal__foot pl-modal__foot--end">{footer}</div> : null}
      </div>
    </div>
  );
}
