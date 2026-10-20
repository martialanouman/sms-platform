Fixed 236px rail on the darkest surface (`--n-950`), grouped by domain, always visible — the console has no hamburger.

```jsx
<SideNav env="PROD" activeId="traffic" onNavigate={go} groups={[
  { label: 'Exploitation', items: [{ id: 'traffic', label: 'Trafic' }, { id: 'sessions', label: 'Sessions', count: 412 }] },
]} />
```

Labels carry the meaning; leave `icon` unset (charte §07 allows no decorative pictograms). Active item = teal tint + teal label. Counts are mono and live.
