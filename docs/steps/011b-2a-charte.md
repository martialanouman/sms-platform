# Step 011b-2a — Charte graphique

Plan : `docs/plan/composants.md` §011, première des deux PR de 011b-2. Arbitrages antérieurs : 1 à 99
(`docs/steps/011a-*.md`, `docs/steps/011b-1-totp-et-elevation.md`). Spec : `docs/spec/08-dashboard.md` §8.8 (WCAG 2.1
AA). Origine : `martialanouman/go-gateway-bo`, skill `.claude/skills/sms-gateway-design/` et sa mise en œuvre sous
`web/src/styles/`. Règles : `.claude/rules/tests.md`. Mode : PR.

## Contexte

- La SPA n'a aucune feuille de style : `/login`, `/access` et `/` sont du HTML nu (011a-4b).
- L'arbitrage 72 (« la charte de design de l'origine n'est pas portée ») a été tranché sans l'humain ; il est révisé
  ici (100).
- Dettes du territoire : #21 (plugin de tokens sans portée, `design-reference.css` dans la feuille d'entrée), fermée en
  011a-4a ; ni le plugin ni cette feuille ne sont portés, elle reste fermée.

**Terminé quand** : sur l'environnement de test, dans un vrai navigateur, `/login`, `/access` et `/` s'affichent dans
la charte (thème sombre, IBM Plex, accent teal) ; les polices viennent de la même origine, sans requête sortante ni
violation de CSP ; l'anneau de focus se voit au clavier ; axe ne relève aucune violation, contraste compris.

## Arbitrages

Fable, 2026-10-20, sauf U à W, tranchés par l'humain le même jour.

100. **La charte est portée** (humain, U), révisant l'arbitrage 72. Le README du skill fait foi pour le contenu et le
     visuel ; où la spec et la charte divergent sur un visuel, la charte l'emporte.
101. **Coupe** (humain, V) : 011b-2a, la charte et les trois écrans existants rhabillés ; 011b-2b, les écrans de
     vérification et d'enrôlement TOTP, sur la charte.
102. **Périmètre** (humain, W) : le skill entier entre tel quel dans `.claude/skills/sms-gateway-design/`, en un commit à
     part, référence des agents et de l'humain ; le code ne reçoit que les tokens et les classes que les écrans
     emploient. Chaque famille suivante arrive, depuis le skill, avec l'écran qui l'emploie. Ni la route `/_design`, ni
     `vite-plugin-tokens`, ni `design-tokens.ts`, ni la couche `.pl-*` entière, ni `@base-ui/react`.
103. **Tokens** : les neuf feuilles de l'ancienne SPA (version auto-hébergée), sans commentaire ; `base.css` sans le
     style propriétaire des barres de défilement ni le lissage de police ; `color-scheme: dark`, aucun thème clair.
     Une graisse déclarée par les tokens est une graisse chargée.
104. **Polices** : `@fontsource/ibm-plex-sans` et `@fontsource/ibm-plex-mono` 5.3.0 (OFL-1.1), sous-ensemble latin, les
     seules graisses que les tokens emploient. Les fichiers sont des assets hachés de même origine : la CSP statique
     les admet, `assetsInlineLimit: 0` les garde hors `data:`. Les `.woff` de repli entrent dans le binaire sans être
     demandés ; accepté.
105. **CSS** : feuilles simples importées par `main.tsx`, classes globales nommées comme la charte (`.ui-*`, `.auth__*`) ;
     ni modules CSS, ni Tailwind, ni CSS-in-JS (la CSP refuse tout `<style>` injecté), aucun style en ligne.
106. **React** : un fichier `web/src/ui.tsx` : `AuthLayout` (la carte d'authentification) et `Field` (libellé, champ,
     erreur liée par `aria-describedby`, ce que login et access câblaient à la main). Les boutons restent des
     `<button>` à classe.
107. **Écrans** : la forme `AuthLayout` de l'ancienne SPA, pas le `LoginScreen` du kit (monogramme, badge et textes
     locaux que l'arbitrage 76 exclut). Textes inchangés. Pendant `Retry-After` (78), le bouton désactivé suit la
     charte (fond enfoncé, texte atténué, `not-allowed`). `/` reste une page sans rail : la coquille arrive avec le
     premier écran qui la peuple.
108. **Navigateurs** : Chrome, Firefox et Safari courants (Vite 8, « baseline widely available ») ; ni polyfill ni
     `browserslist`. L'infobulle (identifiant derrière un libellé français) arrive avec le premier libellé qui en
     remplace un ; jamais `title` seul.

## Design arrêté

- **`.claude/skills/sms-gateway-design/`** : copie intégrale du skill de l'origine, premier commit.
- **`web/package.json`** : `@fontsource/ibm-plex-sans` et `@fontsource/ibm-plex-mono`.
- **`web/src/styles/tokens/*.css`** (neuf feuilles) et **`web/src/styles/app.css`** (`.ui-button`, `.ui-field`,
  `.ui-input`, `.auth__*`), importées par `main.tsx`.
- **`web/src/ui.tsx`** : `AuthLayout`, `Field`.
- **`web/src/routes/login.tsx`, `access.tsx`, `index.tsx`** : rhabillés.
- **Tests** :
  - `web/src/styles.test.ts` : toute `var(--x)` des feuilles est déclarée sous `:root` d'un token, et aucun token
    n'est déclaré hors de `tokens/` ; toute graisse employée est chargée.
  - contraste WCAG AA, calculé sur les valeurs des tokens, pour les paires que les écrans emploient (jsdom ne calcule
    pas les couleurs, et axe y garde `color-contrast` désactivée) ;
  - les tests d'écran existants, inchangés dans leurs attentes.

  Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium par `port-forward` : les trois écrans dans la charte, captures jointes ;
  polices servies depuis l'origine (`immutable`), aucune requête hors origine ; console sans violation de CSP ;
  focus visible au clavier ; axe dans le vrai navigateur, `color-contrast` comprise.
