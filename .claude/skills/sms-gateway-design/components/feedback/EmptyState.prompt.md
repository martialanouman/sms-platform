Explicit empty and unavailable states — including the console's "graceful degradation" states (non stocké, expiré, effacé, non autorisé, module désactivé).

```jsx
<EmptyState inline icon="lock" title="Corps non autorisé">
  La lecture du corps requiert la permission content:read. Chaque accès est journalisé.
</EmptyState>
```

An empty region always says WHY it is empty. Never render a blank area.
