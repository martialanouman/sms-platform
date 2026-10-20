Live operational state. Two forms, chosen automatically by `state`: **dot + mono label** for `link_status`, **tinted pill** for `breaker_state`.

```jsx
<StatusPill state="up" live meta="3/4 binds" />
<StatusPill state="reconnecting" />
<StatusPill state="half_open" note="test de reprise" />
```

Never derive both dimensions from one field, and never render a breaker as a dot: an open breaker on a live link (wait for recovery) and a dead bind (manual rebind) are opposite actions (charte §06).
