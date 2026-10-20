import React from 'react';

export function KeyValueList({ items = [], mono = false, rows = false, className = '' }) {
  const cls = ['pl-kv', mono ? 'pl-kv--mono' : '', rows ? 'pl-kv--rows' : '', className].filter(Boolean).join(' ');
  return (
    <dl className={cls}>
      {items.map((it, i) => (
        <React.Fragment key={it.key ?? i}>
          <dt>{it.label}</dt>
          <dd>{it.value}</dd>
        </React.Fragment>
      ))}
    </dl>
  );
}
