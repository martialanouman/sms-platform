# assets/

No binary brand files were supplied. Everything the brand needs is reproducible from CSS:

- **Mark**: the `SG` monogram on a teal gradient tile (`linear-gradient(140deg, --teal-300, --teal-700)`), as it appears in the charter PDF. Rendered by `SideNav` (26px tile) and `thumbnail.html`; no SVG or PNG file exists. Radius scales with size (7px at 34px, 9px at 44px, 44px at 200px).
- **Wordmark**: "SMS Gateway" in IBM Plex Sans semibold, optionally followed by "· Admin & Exploitation" in muted grey.
- **Icons**: no icon set, no font, no CDN — the charter allows only geometric functional glyphs, drawn inline in `components/core/Icon.jsx` (§07: « Pas de pictogrammes décoratifs ni d'emoji »).
- **Fonts**: IBM Plex Sans + IBM Plex Mono, loaded from Google Fonts in `tokens/fonts.css`. The charter names both families explicitly; only the binaries are missing.
- **Imagery**: the console uses none by design — flat cold near-black surfaces only.

Source: `uploads/Design gp-gateway.pdf` (Charte graphique & design system, v1.0, 5 pages).
