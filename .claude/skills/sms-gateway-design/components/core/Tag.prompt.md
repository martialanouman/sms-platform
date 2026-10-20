Removable chip for applied filters, sender IDs, keywords and group membership.

```jsx
<Tag mono onRemove={() => drop('BANQUEX')}>BANQUEX</Tag>
<Tag>Groupe · Banques CI</Tag>
```

Use `mono` for any operator-typed identifier. Static tags (no `onRemove`) read as metadata, not affordances.
