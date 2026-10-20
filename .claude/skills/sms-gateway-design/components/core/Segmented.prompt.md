Mutually exclusive toggle for a small closed set (charte §05): time window (5 min / 1 h / 24 h), bind side (SMSC / utilisateur), traffic direction.

```jsx
<Segmented value={win} onChange={setWin} items={['5 min', '1 h', '24 h']} />
```

Two to four items. Beyond that use a Select; for navigation between resource sub-views use Tabs.
