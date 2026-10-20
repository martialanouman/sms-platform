Dense sticky-header table behind every list in the console: CDR results, sessions, routes, suppressions, audit log.

```jsx
<DataTable dense selectedKey={sel} onRowClick={(r) => setSel(r.id)}
  columns={[
    { key: 'ts', header: 'Horodatage', mono: true, width: 150 },
    { key: 'dest', header: 'Destination', mono: true },
    { key: 'status', header: 'Statut', render: (r) => <StatusPill state={r.status} /> },
    { key: 'credits', header: 'Crédits', numeric: true },
  ]}
  rows={rows} empty={<EmptyState icon="search-x" title="Aucun message" />} />
```

Numeric columns are always right-aligned and monospace. Row click opens a detail panel; it never navigates away.
