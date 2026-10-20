Exclusive policy choice where each option needs its consequence spelled out — content storage policy, erasure target, balance scope.

```jsx
<RadioGroup name="policy" value="stored_encrypted" onChange={set} options={[
  { value: 'off', label: 'Ne pas stocker', description: 'Aucun corps conservé. Le CDR reste complet.' },
  { value: 'stored_encrypted', label: 'Stocké chiffré', description: 'Protège le repos ; content:read reste la frontière d\'accès.' },
]} />
```
