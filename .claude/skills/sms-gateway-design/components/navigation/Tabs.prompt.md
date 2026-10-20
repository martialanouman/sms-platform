Sub-navigation inside one resource — the SMPP account page (Identifiants, Canaux, Quotas, Webhooks, Sessions) or the session monitor (binds utilisateur / SMSC).

```jsx
<Tabs activeId="creds" onChange={set} tabs={[
  { id: 'creds', label: 'Identifiants', icon: 'key-round' },
  { id: 'sessions', label: 'Sessions', count: 8 },
]} />
```

Disable rather than hide a tab the operator's permissions block, so the surface stays legible.
