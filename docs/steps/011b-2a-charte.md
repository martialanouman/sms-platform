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

Fable, 2026-10-20, sauf U à Y, tranchés par l'humain le même jour (X et Y au tour de revue).

100. **La charte est portée** (humain, U), révisant l'arbitrage 72. Le README du skill fait foi pour le contenu et le
     visuel ; où la spec et la charte divergent sur un visuel, la charte l'emporte.
101. **Coupe** (humain, V) : 011b-2a, la charte et les trois écrans existants rhabillés ; 011b-2b, les écrans de
     vérification et d'enrôlement TOTP, sur la charte.
102. **Périmètre** (humain, W) : le skill entier entre tel quel dans `.claude/skills/sms-gateway-design/`, en un commit à
     part, référence des agents et de l'humain ; le code reçoit les neuf feuilles de tokens entières (humain, X, entre
     cette phrase et 103) et seulement les classes que les écrans emploient. Chaque famille de classes suivante arrive,
     depuis le skill, avec l'écran qui l'emploie. Ni la route `/_design`, ni
     `vite-plugin-tokens`, ni `design-tokens.ts`, ni la couche `.pl-*` entière, ni `@base-ui/react`.
103. **Tokens** : les neuf feuilles de l'ancienne SPA (version auto-hébergée), sans commentaire ; `base.css` sans le
     style propriétaire des barres de défilement ni le lissage de police ; `color-scheme: dark`, aucun thème clair.
     Une graisse déclarée par les tokens est une graisse chargée, en sans comme en mono.
104. **Polices** : `@fontsource/ibm-plex-sans` et `@fontsource/ibm-plex-mono` 5.3.0 (OFL-1.1), sous-ensemble latin, les
     seules graisses que les tokens emploient. Les fichiers sont des assets hachés de même origine : la CSP statique
     les admet, `assetsInlineLimit: 0` les garde hors `data:`. Les `.woff` de repli entrent dans le binaire sans être
     demandés ; accepté.
105. **CSS** : feuilles simples importées par `main.tsx`, classes globales nommées comme la charte (`.ui-*`, `.auth__*`) ;
     ni modules CSS, ni Tailwind, ni CSS-in-JS (la CSP refuse tout `<style>` injecté), aucun style en ligne.
106. **React** : un fichier `web/src/ui.tsx` : `AuthLayout` (la carte d'authentification) et `Field` (libellé et champ
     liés par `useId`). L'alerte reste dans l'écran, qui la lie aux champs par `aria-describedby`. Les boutons restent
     des `<button>` à classe.
107. **Écrans** : la forme `AuthLayout` de l'ancienne SPA, pas le `LoginScreen` du kit (monogramme, badge et textes
     locaux que l'arbitrage 76 exclut). Textes inchangés. Pendant `Retry-After` (78), le bouton désactivé suit la
     charte (fond enfoncé, texte atténué, `not-allowed`). `/` reste une page sans rail : la coquille arrive avec le
     premier écran qui la peuple.
108. **Navigateurs** : Chrome, Firefox et Safari courants (Vite 8, « baseline widely available ») ; ni polyfill ni
     `browserslist`. L'infobulle (identifiant derrière un libellé français) arrive avec le premier libellé qui en
     remplace un ; jamais `title` seul.

109. **AA prime sur la charte pour la limite d'un champ** (humain, Y) : le contour de la charte (n-600) ne fait que
     1,3:1 sur une carte, sous les 3:1 de WCAG 1.4.11 ; `--border-field` (n-300) et `--border-field-hover` (n-200),
     pris dans la palette de la charte, les atteignent. Troisième écart assumé, avec `--n-300` et `--red-400`.

## Design arrêté

- **`.claude/skills/sms-gateway-design/`** : copie intégrale du skill de l'origine, premier commit.
- **`web/package.json`** : `@fontsource/ibm-plex-sans` et `@fontsource/ibm-plex-mono`.
- **`web/src/styles/tokens/*.css`** (neuf feuilles) et **`web/src/styles/app.css`** (`.ui-button`, `.ui-field`,
  `.ui-input`, `.auth__*`), importées par `main.tsx`.
- **`web/src/ui.tsx`** : `AuthLayout`, `Field`.
- **`web/src/routes/login.tsx`, `access.tsx`, `index.tsx`** : rhabillés.
- **Tests** :
  - `web/src/styles/styles.test.ts` : toute `var(--x)` des feuilles est déclarée sous `:root` d'un token, et aucun token
    n'est déclaré hors de `tokens/` ; toute graisse employée est chargée.
  - contraste WCAG AA, calculé sur les valeurs des tokens, pour les paires que les écrans emploient (jsdom ne calcule
    pas les couleurs, et axe y garde `color-contrast` désactivée) ;
  - les tests d'écran existants, inchangés dans leurs attentes.

  Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium par `port-forward` : les trois écrans dans la charte, captures jointes ;
  polices servies depuis l'origine (`immutable`), aucune requête hors origine ; console sans violation de CSP ;
  focus visible au clavier ; axe dans le vrai navigateur, `color-contrast` comprise.

## Écarts au design, constatés à l'implémentation

- **Vitest traite les feuilles de `src/styles/`** (`test.css.include`) : sans cela, il vide toute feuille, lecture brute
  (`?raw`) comprise, et les tests de la charte ne liraient rien. Le projet n'a pas les types de Node : les tests lisent
  les feuilles par `import.meta.glob`, pas par `fs`.
- **Deux commentaires de valeur survivent dans `colors.css`** : les écarts assumés de l'origine à la charte
  (`--n-300` éclairci, `--red-400` ajouté), sans lesquels on « corrigerait » une valeur vers une couleur qui échoue AA.
- **Le bouton désactivé** se sélectionne par `:disabled` natif, et non par les attributs de `@base-ui/react` de
  l'origine.
- **`/`** emploie la même carte que la connexion, en attendant la coquille.

## Mutations

Douze, à la main : un token mal nommé, un token déclaré hors des tokens, une graisse non chargée (sans et mono), une
graisse déclarée sans police, quatre couleurs assombries sous AA, l'import de la feuille retiré de `main.tsx`, le lien
du libellé au champ coupé de deux façons. Toutes tombent.

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.15.0 : image construite par GoReleaser en snapshot,
importée sous `proof-011b2a`, timer suspendu, puis réarmé et image de la release rétablie. Dans Chromium
(`agent-browser`), par tunnel SSH vers `port-forward`, sur `localhost:8080` :

1. `/login` ([capture](011b-2a/connexion.png)) : fond `rgb(12, 15, 20)`, carte `rgb(17, 22, 30)`, titres en IBM Plex
   Sans ; polices chargées 400, 500 et 600 ; douze requêtes, toutes vers l'origine ; ni erreur de console ni violation
   de CSP ; axe-core injecté dans la page : aucune violation, `color-contrast` comprise.
2. Refus ([capture](011b-2a/refus.png)) : bandeau rouge teinté, axe sans violation ; focus clavier
   ([capture](011b-2a/focus.png)) : anneau `0 0 0 2px` couleur de page, `0 0 0 4px` teal.
3. `/access` sans jeton ([capture](011b-2a/lien-incomplet.png)) et `/` après connexion ([capture](011b-2a/accueil.png)) :
   axe sans violation.
4. Une police, `ibm-plex-sans-latin-400-normal-….woff2` : 200, `font/woff2`, `public, max-age=31536000, immutable`.

## Correctifs de revue, tour 1

Trois revues (correction et accessibilité ; tests creux ; code en trop). Aucun bloquant ; 19 constats, dont deux tranchés
par l'humain.

1. **Arbitrage 102 reformulé** (humain, X) : il contredisait 103 ; les feuilles de tokens restent entières.
2. **Arbitrage 109** (humain, Y) : contour de champ à 3:1. La revue proposait n-400 ; le test l'a mesuré à 2,4:1, et
   n-300 a été retenu.
3. **Focus en couleurs forcées** : `outline: 2px solid transparent` au lieu de `none`, que le mode Contraste élevé
   rend visible quand il retire les ombres. jsdom ne le voit pas ; la preuve relève `outline-style: solid`.
4. **Carte** : contour `--border-default`, celui de la charte ; le padding de 32 px reste celui de la forme
   d'authentification de l'origine.
5. **Retraits** : `--qr-paper` (absent du skill, il reviendra avec l'écran du QR, en 011b-2b), le `gap` d'un bouton
   sans enfant à espacer, la paire de contraste d'un lien qu'aucun écran ne rend.
6. **Tests** :
   - déclarations sous `:root` seulement, et une seule fois sur l'ensemble des feuilles (un token redéclaré dans une
     feuille lue plus tard échappait au test de contraste) ;
   - `app.css` importe chaque feuille de tokens ;
   - mélange de couleurs ancré sur deux valeurs connues (le mélange non prémultiplié passait) ;
   - paires réellement rendues : secondaire sur carte, survols, bouton désactivé (garde l'écart de `--n-300`) ;
   - limite des champs à 3:1 et son câblage dans `app.css` ;
   - graisses mono égales aux graisses déclarées.
7. **Gardés tels quels** : `min-width` et `white-space` des champs et boutons (la charte) ; les captures du refus et
   du focus, que la preuve décrit.

Mutations du tour : dix, toutes tombent. Preuve rejouée sur `proof-011b2a-2`, captures remplacées : contour des champs
`rgb(132, 143, 158)`, focus `outline-style: solid` et anneau teal, axe sans violation sur les cinq états, quarante et une
requêtes, toutes vers l'origine.
