import React from 'react';
import { Icon } from '../core/Icon.jsx';

export function DataTable({
  columns = [], rows = [], rowKey = (r, i) => r.id ?? i, dense = false,
  selectedKey, onRowClick, sort, onSortChange, empty, className = '',
}) {
  const cellCls = (c) => ['pl-td', c.numeric ? 'pl-td--num' : '', c.mono ? 'pl-td--mono' : '', c.muted ? 'pl-td--muted' : '']
    .filter(Boolean).join(' ') || undefined;
  return (
    <div className={`pl-tablewrap ${className}`.trim()}>
      <table className={`pl-table${dense ? ' pl-table--dense' : ''}`}>
        <thead>
          <tr>
            {columns.map((c) => (
              <th
                key={c.key}
                className={[c.numeric ? 'pl-th--num' : '', c.sortable ? 'pl-th--sortable' : ''].filter(Boolean).join(' ') || undefined}
                style={c.width ? { width: c.width } : undefined}
                onClick={c.sortable && onSortChange ? () => onSortChange(c.key) : undefined}
              >
                {c.header}
                {c.sortable && sort && sort.key === c.key ? (
                  <Icon name={sort.dir === 'asc' ? 'chevron-up' : 'chevron-down'} size={12} style={{ marginLeft: 4, verticalAlign: 'middle' }} />
                ) : null}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr><td colSpan={columns.length} style={{ padding: 0 }}>{empty}</td></tr>
          ) : rows.map((r, i) => {
            const k = rowKey(r, i);
            return (
              <tr
                key={k}
                className={[selectedKey === k ? 'is-selected' : '', onRowClick ? 'is-clickable' : ''].filter(Boolean).join(' ') || undefined}
                onClick={onRowClick ? () => onRowClick(r, k) : undefined}
              >
                {columns.map((c) => (
                  <td key={c.key} className={cellCls(c)}>{c.render ? c.render(r, i) : r[c.key]}</td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
