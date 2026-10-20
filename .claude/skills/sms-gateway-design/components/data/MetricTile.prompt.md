Single real-time counter for the traffic dashboard — MT/s, MO/s, success rate, p99 latency, active sessions.

```jsx
<MetricTile label="MT / s" value="8 123" tone="mt" live delta="+4,2 %" />
<MetricTile label="Latence p99" value="212" unit="ms" />
```

Numbers are monospace with a French thin space as thousands separator. MT is blue, MO is teal — never the same colour.
