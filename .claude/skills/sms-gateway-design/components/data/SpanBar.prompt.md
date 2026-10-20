One row of the SMS trace waterfall — ingestion, sender-ID authorisation, opt-out, anti-spam, routing, throttling, billing, submit, DLR, delivery.

```jsx
<SpanBar name="anti-spam" startPct={12} widthPct={6} duration="4 ms" state="ok" />
<SpanBar name="facturation" depth={1} startPct={18} widthPct={22} duration="61 ms" state="slow" />
```

The message body NEVER appears in a trace. Failed and slow spans are colour-flagged.
