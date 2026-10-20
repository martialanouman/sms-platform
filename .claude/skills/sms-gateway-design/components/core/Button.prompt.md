The action control for every form, toolbar and modal footer; `md` (32px) is the default, `sm` (26px) for table row actions.

```jsx
<Button variant="primary" icon="plus">Nouveau client</Button>
<Button variant="secondary" size="sm" icon="rotate-cw">Rebind</Button>
<Button variant="danger" icon="power">Déconnecter la session</Button>
```

Buttons are **contour + teinte, never a solid fill** (charte §05): primary is teal-outlined, destructive red-outlined. One primary per view. Destructive acts that need an audit trail (rotation, levée de suppression, effacement) use `danger`; the same act inside a table row uses `dangerGhost` + `size="sm"`.
