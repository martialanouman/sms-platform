Credential card for an SMPP account: always masked, no reveal action, rotation impact visible up front.

```jsx
<MaskedSecret kind="Clé API" last4="7F2A" lastUsedAt="il y a 2 min" liveSessions={8}
  actions={<><Button size="sm" variant="secondary" icon="rotate-cw">Rotation</Button><Button size="sm" variant="dangerGhost" icon="ban">Révoquer</Button></>} />
```

Render exactly two of these per account (SMPP bind identifier + API key) — never an extensible list.
