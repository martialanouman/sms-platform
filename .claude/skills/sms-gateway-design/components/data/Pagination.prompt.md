Cursor-based pager — the CDR API paginates by cursor, so there are no page numbers, only Précédent / Suivant.

```jsx
<Pagination range="1 – 100" total="≈ 24 380" pageSize={100} onPageSizeChange={setSize} hasNext onNext={next} note="Fraîcheur ~15 s" />
```

Never render numbered pages for cursor endpoints.
