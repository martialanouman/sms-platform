The billing screen's teaching device: two visually distinct cards so nobody reads MO as a blocking balance.

```jsx
<BalanceCard direction="mt" value={12450} meterPct={62} scope="Pool partagé"
  note="Bloque à zéro en prépayé sans découvert." actions={<Button size="sm" icon="plus">Recharger</Button>} />
<BalanceCard direction="mo" value={3120} unit="crédits consommés" scope="Pool partagé"
  note="Le MO est toujours remis : un dépassement MO ne bloque jamais vos envois MT." />
```

MT is blue and rechargeable; MO is teal, has no top-up action, and always carries its note.
