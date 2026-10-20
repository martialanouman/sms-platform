import React from 'react';
import { Button } from '../core/Button.jsx';

export function Pagination({ range, total, pageSize, onPageSizeChange, hasPrev = false, hasNext = false, onPrev, onNext, note, className = '' }) {
  return (
    <div className={`pl-pager ${className}`.trim()}>
      <span>{range}{total ? <> · <strong style={{ fontWeight: 500 }}>{total}</strong></> : null}</span>
      {note ? <span>{note}</span> : null}
      <div className="pl-pager__spacer">
        {pageSize && onPageSizeChange ? (
          <select className="pl-select pl-select--sm" style={{ width: 74 }} value={pageSize} onChange={(e) => onPageSizeChange(Number(e.target.value))}>
            {[50, 100, 250, 500].map((n) => <option key={n} value={n}>{n} / page</option>)}
          </select>
        ) : null}
        <Button size="sm" variant="secondary" icon="chevron-left" disabled={!hasPrev} onClick={onPrev}>Précédent</Button>
        <Button size="sm" variant="secondary" iconAfter="chevron-right" disabled={!hasNext} onClick={onNext}>Suivant</Button>
      </div>
    </div>
  );
}
