Loading state for surfaces whose structure is already known — tables, cards, virtualised rows (charte §08). It reproduces the real layout; it is never a full-screen spinner.

```jsx
<SkeletonRows rows={6} columns={[150, 130, 90, 60]} dense />
```

Spinners are only for a single punctual action (a button's `loading`); real-time surfaces use the discreet « live » indicator instead.
