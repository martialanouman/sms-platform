Incoming notification from the multiplexed WS socket (alerts, billing floors, session events). Dark chrome surface, bottom-right stack.

```jsx
<ToastStack>
  <Toast severity="critical" title="orange-ci : taux d'erreur > 5 %" source="alertmanager" onClose={dismiss} />
</ToastStack>
```

Always name the `source` — operators need to know whether detection came from Alertmanager (independent of dashboard uptime) or the BFF evaluator.
