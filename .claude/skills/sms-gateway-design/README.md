# SMS Gateway — Design System

Design system for the **SMS Gateway admin & exploitation console**: the internal, data-dense, desktop-first operations tool that runs customers, SMPP accounts, connectors, routing, compliance and billing for an SMPP + REST SMS gateway.

Everything here follows **`uploads/Design gp-gateway.pdf` — « Charte graphique & design system · SMS Gateway · Admin & Exploitation », v1.0** (dark theme, desktop-first, WCAG 2.1 AA). Where the charter and the earlier technical spec disagree on a visual, **the charter wins**.

## Sources given

| Source | What it provided |
|---|---|
| `uploads/Design gp-gateway.pdf` (v1.0, 5 pages, 10 sections) | The visual system: brand mark, palette with exact hexes, type scale, spacing/radii/layout, component specimens, status semantics, data-viz, content states, pagination, feedback. **Ground truth.** |
| `uploads/specification-technique-tableau-de-bord.md` (v2.0, RESHADED) | The product: 30+ functional surfaces, permission catalogue, BFF API surface (§5.1), per-screen detail (§6), NFRs. French narrative, English code. |
| `specification-technique-passerelle-sms.md` (companion, **not supplied**) | Cited ~20× by the spec (§6.x compagnon). Never provided. |
| `@martialanouman/gateway-api-contracts`, `api/openapi-admin.yaml` (134 operations) | Mentioned; **no access**. Endpoint shapes come from §5.1 of the spec. |

No codebase and no Figma file were provided. The charter's rendered pages were read directly from the PDF (text + rendered pages) — not from memory.

## Product context

- **One product, one audience.** Internal tool for 100–300 concurrent operators. Customers have no access at all (*« Les clients n'ont aucun accès à la plateforme »*), so there is exactly **one UI kit**.
- **Control-room ambience.** The charter's four design principles: *densité maîtrisée* (lots of data, but hierarchised), *statut sans ambiguïté* (one colour + one label per critical state), *sombre & reposant* (cold near-black for long watches), *un seul accent* (teal marks action and life; everything else stays neutral so alerts are not drowned).
- **Two-level domain model.** A *client* holds one or more *comptes SMPP*; a *groupe* is organisational only and carries no configuration.
- **Real-time by nature.** Metrics, sessions and notifications arrive on one multiplexed WebSocket (2–5 s visible freshness). Live values are marked with the pulsing dot; snapshots are not.
- **Reliability is stratified.** Infra alerts are evaluated by Alertmanager independently of the dashboard; business alerts by the BFF on a durable source. The UI states which one detected — blue for `alertmanager`, violet for `bff`.
- **Permission-based, not role-based.** The UI renders from `/auth/me`'s permission set. Blocked controls are disabled and explained, never silently hidden.
- **Compliance is foreground.** Message bodies, credential secrets, opt-out lifting and RGPD erasure are gated, audited, and always shown with their consequence in plain French.

---

## CONTENT FUNDAMENTALS

**Language.** UI copy is **French**; technical identifiers stay **English and monospace**, verbatim from the API: `link_status`, `breaker_state`, `max_sessions`, `balance_scope`, `mo_billing_floor`, `content:read`, `query_sm`, `weighted`, `resolveRoute()`. A French label **may stand for** an identifier — `reconnecting` shown as « Reconnexion » — but it never **replaces** it: the identifier stays reachable right next to the label, through a focusable trigger whose tooltip is wired with `aria-describedby` and shown **on focus as well as hover**. `title` alone does not qualify — it never opens for a keyboard user, and an operator who cannot see the identifier can no longer match the screen to what they grep in the logs.

**Mono means machine.** The charter is explicit: *« Le mono est réservé aux valeurs machine : identifiants, compteurs, MSISDN, sender IDs, états techniques. Jamais pour du texte narratif. »* So `msg_01J9K2A7QF`, `2250701020304`, `8 123 MT/s`, `breaker: half_open` are mono; sentences never are.

**Person.** Third person and impersonal constructions — the interface describes the system, not the operator: *« Baisser ce quota ne coupe pas les binds vivants. »* Second person appears only where the operator accepts a consequence (*« Je comprends que le nouveau secret ne sera affiché qu'une seule fois. »*). First person: never.

**Consequence-first.** Every ambiguous state states what it means, what it does, and what it does *not* do:

- *« Le MO est toujours remis : un dépassement MO ne bloque jamais vos envois MT. »*
- *« Ces règles priment sur les scripts et le matching déclaratif ; le court-circuit ne saute que la résolution de route. »*
- *« Bac à sable : timeout 50 ms · mémoire 8 Mo. En cas d'échec, le routage déclaratif prend le relais. »*
- *« Un unbind gracieux sera envoyé à ses_9f2a…. Le compte devra se reconnecter. »*
- *« Module désactivé sur la passerelle. Dégradation propre — jamais une erreur. »*

**Honesty over reassurance.** Encryption protects data at rest; `content:read` remains the access boundary — say so. Never write "sécurisé" as a claim.

**Empty ≠ error ≠ disabled.** Five distinct states, five distinct copies (charte §08): *chargement* (skeleton reproducing the real layout), *état vide* (nothing yet + how to create), *aucun résultat* (filters too narrow + how to widen), *module désactivé* (graceful degradation), *erreur* (HTTP reality + « vos données locales restent affichées » + Réessayer).

**Casing.** Sentence case for labels, titles and buttons. Micro-labels (10.5px) are UPPERCASE with +0.08em tracking. Technical status pills (link, breaker, delivery) keep the API's own lowercase snake_case (`half_open`, `reconnecting`) because that is what the payload says and what operators grep in logs. Administrative statuses (customer, account, group, sender name) are shown in French (`Actif`, `Suspendu`, `Fermé`).

**Buttons are verbs, and destructive buttons name the act:** *« Effectuer la rotation »*, *« Déconnecter »*, *« Lever le désabonnement »*, *« Lancer le job »*, *« Réessayer »* — never *« OK »* or a bare *« Confirmer »*.

**Numbers.** French formatting: space thousands separator (`504 219`), comma decimal (`99,64 %`), unit always present (`crédits SMS`, `MT/s`, `ms`, `1,24 s`). Approximations are marked (`~1,24 M au total`), as is freshness (`fraîcheur ~15 s`).

**Audit is announced before the act**, in plain French and next to the trigger, not after the fact: *« L'action est enregistrée dans le journal d'audit. »*, *« La lecture est enregistrée dans le journal d'audit. »*

**No emoji, ever.** Charter §07: *« Pas de pictogrammes décoratifs ni d'emoji. »*

**Tone in one line:** a calm senior network engineer telling you exactly what is about to happen, in the fewest words that stay precise.

---

## VISUAL FOUNDATIONS

**Intent.** A dark control room. Four cold near-black surfaces, one teal accent, and a rigid status semantic. Chrome recedes; numbers and states are the content.

**Colour (exact charter values).**
- Surfaces: `--surface-page #0c0f14` (canvas), `--surface-chrome #0a0d12` (sidebar/topbar), `--surface-card #11161e` (cards, panels), `--surface-sunken #0c1017` (fields, insets).
- Borders: `--border-default #232b36` (card contours), `--border-subtle #1c242f` (separators). On near-black, **the border carries depth, not the shadow.**
- Text: `--text-primary #e6edf3`, `--text-muted #8b95a3`. Two levels, plus `--text-faint` for mono metadata.
- Accent: **teal `#2dd4bf`** — action, selection, principal metric, anything alive. Exactly one accent, deliberately.
- Status: green `#2fd18a` (sain, up, closed, delivered), amber `#f0b84a` (dégradé, reconnecting, breaker open, expired), red `#e5484d` (panne, failed, suspended, destructive).
- Domain: blue `#7fb2ff` (secondary metric, technical types, `alertmanager`), violet `#c586c0` (business domain: réputation, facturation, BFF evaluation, routing scripts).
- **Direction is a colour:** MT is teal (principal), MO is blue (secondary). Never swapped.
- Tints are the same colour at ~14 % over the surface (`--tint-*`); a tinted background only ever carries text of its own family. **No light theme exists** — the charter is dark, full stop.

**Typography.** IBM Plex Sans + IBM Plex Mono. Six sans roles and nothing else: **28/600** page title & KPI value, **19/600** section, **15/600** card, **13/400** body, **12/400** secondary label, **10.5/600 caps +0.08em** micro-label. Mono at 11–13px for data, 28/600 for metrics. `font-variant-numeric: tabular-nums` is global so streaming columns never dance.

**Spacing.** Base 4px; canonical steps **4 / 8 / 12 / 16 / 24 / 40**. Cells 8×12, cards 16, panels 24, section gaps 40.

**Radii.** Three: **7px** fields/buttons, **12px** cards/modals/banners, **20px** pills/segmented. Dots and avatars are fully round. Nothing else.

**Layout.** Fixed shell, scrolling content: nav rail **236px**, topbar **56px**, sub-bar 44px (tabs/filters), detail panel 420px, content capped at 1600px. Rows 38px (32px dense); controls 28 / 34 / 40px.

**Buttons.** **Contour + teinte, never a solid fill** — the charter states it for the destructive case (*« toujours en contour, jamais plein »*) and the specimens show the same for primary. Primary = teal border + 14 % teal fill + teal label; secondary = neutral border; destructive = red border + red label; link = bare teal.

**Status semantics (the strictest rule in the system).** `link_status` renders as a **coloured dot + mono label**; `breaker_state` renders as a **tinted pill**. They are never merged and never derived from one field: *« Un disjoncteur ouvert sur un lien vivant (attendre la reprise) et un bind mort (rebind manuel) demandent des actions opposées. »*

**Cards.** `--surface-card` on `--surface-page`, 1px `--border-default`, 12px radius, no shadow. KPI cards are a **single surface** — micro-label + mono 28 value + signed delta on `--surface-card`, with no nested inset (a second dark block on near-black reads as a double background). No coloured left-border accent cards anywhere.

**Shadows.** Only floating layers get one: popovers/toasts `0 8px 24px rgba(0,0,0,.5)`, modals `0 24px 64px rgba(0,0,0,.6)`. Cards get none.

**Transparency & blur.** The modal scrim (`rgba(6,8,11,.72)`) and the colour tints. No frosted panels — blurring live metrics behind a dialog costs more than it gives.

**Motion.** 120–240 ms with IBM productive/entrance/exit easings. **One looping animation in the whole system:** the 1.8 s pulse of the live dot. Skeletons shimmer at 1.4 s. Real-time counters swap value with no transition. Spinners only for a single punctual action — never full-screen. No bounce, no parallax; `prefers-reduced-motion` disables everything.

**Interaction states.** Hover = stronger tint / `--surface-hover`; press = stronger still; **no scale, no translation** (a mis-click here disconnects a live bind). Focus = 2px page-colour spacer + 2px teal ring on every interactive element. Disabled = sunken fill + `--text-faint` + `not-allowed`; permission-blocked controls are disabled and explained, never removed.

**Tables.** Sticky micro-caps headers on `--surface-card`, mono values, right-aligned numerics, 76px teal share meters, hover lightening, selected row in teal tint. Tens of thousands of rows are virtualised: only the visible window renders, the rest arrive as skeleton rows on scroll. Pagination is **cursor-based** — Précédent / Suivant with a row count and total, never page numbers.

**Data-viz.** Teal area (18 % fill) + 1.4px teal line for the principal series, thin blue line for the secondary, two discreet grid lines, no axes. Legends are 8px square markers + mono labels. One filled area per chart.

**Imagery.** None, by design: no photography, illustration, texture, pattern or decorative gradient. The single gradient in the system is the monogram tile (`140°`, teal-300 → teal-700).

---

## ICONOGRAPHY

The charter closes this question: *« Pas de pictogrammes décoratifs ni d'emoji. Formes géométriques simples et glyphes fonctionnels uniquement. »*

- **There is no icon library, no icon font and no CDN.** The complete set is drawn inline in `components/core/Icon.jsx`: `dot`, `square` (legend marker), `diamond`, `circle`, `warning`, `bang`, `info`, `plus`, `minus`, `times`, `check`, `chevron-up|down|left|right`, `arrow-up`, `arrow-down`, `refresh`, `search`, `ban`, `ellipsis`, `ellipsis-vertical`, `bell` (the notification centre's icon-only trigger, added 29/09/2026).
- **A name outside the set renders nothing** — deliberately. If a control seems to need an unavailable glyph, the answer is its text label.
- **Sizes:** 14px in controls and rows, 16px in headers and empty states, 18px in specimens. Never under 12px. Stroke 1.5, `currentColor`, no fills except the dot/square/ellipsis markers.
- **Where glyphs are allowed:** status dot (the most-used glyph in the product), legend square, menu chevrons, warning triangle, the boxed glyph inside an empty/error state, and icon-only controls that carry a tooltip (close, more, refresh).
- **The nav rail has no icons at all** — labels carry the meaning.
- **Brand mark:** the `SG` monogram on the teal gradient tile, plus the "SMS Gateway" wordmark in Plex Sans semibold. No logo file was supplied; the mark is reproduced in CSS (`.pl-nav__logo`, `thumbnail.html`).

---

## Index

**Root** — `styles.css` (the only file consumers link; `@import` list only), `README.md`, `SKILL.md`, `thumbnail.html`.

**Tokens** (`tokens/`) — `fonts.css` (IBM Plex families), `colors.css` (charter hexes, tints, semantic + domain aliases), `typography.css` (six roles + mono roles), `spacing.css`, `radius.css` (7/12/20), `elevation.css`, `motion.css`, `layout.css` (236/56/44/420), `base.css` (resets, dark `color-scheme`, focus ring, scrollbars, reduced-motion).

**Component styles** — `components/components.css` (the `.pl-*` layer, imported by `styles.css`).

**Components** (`components/<group>/`, each with `.jsx` + `.d.ts` + `.prompt.md`; one `@dsCard` per directory)
- `core/` — **Button**, **IconButton**, **Icon** (+ **Dot**), **Badge**, **Tag**, **StatusPill**, **Segmented**, **Card**
- `forms/` — **Field**, **Input**, **Textarea**, **Select**, **Checkbox**, **Switch**, **RadioGroup**
- `data/` — **DataTable**, **MetricTile**, **KeyValueList**, **SpanBar**, **Pagination**
- `feedback/` — **Banner**, **Modal**, **EmptyState**, **Skeleton** (+ **SkeletonRows**), **ErrorState**, **Toast** (+ **ToastStack**)
- `navigation/` — **SideNav**, **TopBar**, **Tabs**
- `domain/` — **BalanceCard**, **MaskedSecret**

*Every family above is specified by the charter (§05 boutons, badges, champs, sélecteurs, segmented, onglets, KPI, tableau; §08 états de contenu; §09 pagination; §10 toasts, confirmation, bannières).* **Intentional additions**: `Icon`/`Dot` (wrapper for the charter's glyph set), `SpanBar`, `BalanceCard`, `MaskedSecret` — the last three come from spec §6.5/§6.11/§6.12/§6.14, each a repeated, easily-misread widget that generic primitives would flatten.

**Templates** (`templates/`) — `ecran-console/EcranConsole.dc.html` ("Écran de console"): the screen scaffold consuming projects start from — nav rail, topbar, filter sub-bar, four live KPI tiles, connector table with both status dimensions. Repoint `ecran-console/ds-base.js`'s `base` line in a consuming project.

**Guidelines** (`guidelines/`) — 23 specimen cards: **Colors** (surfaces & texte, accent & statut, bleu/violet métier, teintes, direction MT/MO), **Type** (échelle sans, mono données, chiffres & formats français), **Spacing** (échelle, rayons, shell applicatif), **Brand** (marque, boutons, sémantique de statut, champs & bascules, tableau, KPI, data-viz, jeu de glyphes, états de contenu, bandeaux & toasts, pagination, mouvement).

**UI kit** (`ui_kits/admin-console/`) — see its `README.md`. `index.html` is the clickable console (login + MFA → trafic → CDR/trace → compte SMPP → routes → facturation); screens in `TrafficScreen.jsx`, `CdrExplorerScreen.jsx`, `AccountScreen.jsx`, `RoutesScreen.jsx`, `BillingScreen.jsx`, `LoginScreen.jsx`, with `AppShell.jsx` (shell, `Chart`, `Legend`, `Toolbar`, `Page`) and `mock.js`.

**Assets** (`assets/`) — no binaries: the mark, the glyphs and the wordmark are all CSS/SVG-in-component. See `assets/README.md`.

**No slide template** was provided, so no sample slides exist.

## Open items

1. **Fonts** — IBM Plex Sans/Mono are the charter's own choice, but loaded from Google Fonts. Send `.woff2` binaries to self-host.
2. **Companion spec** — `specification-technique-passerelle-sms.md` is cited ~20× and was never supplied.
3. **Unmocked surfaces** — anti-spam, désabonnements, numéros entrants, contenu/RGPD, opérateurs/rôles, journal d'audit and the Monaco script editor are specified in prose but have no visual reference; the kit shows an explicit empty state rather than inventing them.
4. **No `Tooltip` component** — the identifier rule above needs one and the kit has none. `IconButton` is the closest thing, and it carries its label through `title`, which that rule rules out. The first screen showing a French label for a technical state — `step-084`, connector health — has to add the component before it can comply.
