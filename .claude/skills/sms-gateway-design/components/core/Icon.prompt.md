The only icon primitive: a closed set of geometric functional glyphs (charte §07 forbids decorative pictograms and emoji). Unknown names deliberately render nothing, so a control falls back to its text label.

```jsx
<Icon name="chevron-down" size={14} />
<Icon name="warning" size={16} />
<Dot tone="up" live />
```

No CDN, no icon font, no dependency. If a screen seems to "need" an icon outside the set, the answer is a text label — not a new glyph.
