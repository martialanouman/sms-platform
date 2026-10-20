The panel container: 1px subtle border, 6px radius, one-pixel shadow. Never a coloured left border.

```jsx
<Card title="Connecteurs" subtitle="12 actifs" actions={<Button size="sm" icon="plus">Ajouter</Button>} flush>
  <DataTable columns={cols} rows={rows} />
</Card>
```

`flush` for table bodies, `sunken` for read-only reference blocks.
