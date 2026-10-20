# Step 011b-2b — Écrans de vérification et d'enrôlement

Plan : `docs/plan/composants.md` §011, seconde des deux PR de 011b-2. Arbitrages antérieurs : 1 à 109
(`docs/steps/011a-*.md`, `docs/steps/011b-1-totp-et-elevation.md`, `docs/steps/011b-2a-charte.md`). Spec :
`docs/spec/08-dashboard.md` §8.3, §8.8. Origine : `martialanouman/go-gateway-bo`, `web/src/routes/mfa.tsx`,
`web/src/components/totp-enrollment.tsx`. Règles : `.claude/rules/tests.md`. Mode : PR.

## Contexte

- Le serveur sait tout depuis 011b-1 : `enrollTotp`, `confirmTotp`, `verifyMfa`, `me` avec `elevated` et
  `secondFactors`. La SPA, elle, ignore le défi rendu par `login` et ouvre `/` sur une session non élevée.
- La charte est portée (011b-2a) ; chaque famille de classes arrive avec l'écran qui l'emploie (102).

**Terminé quand** : sur l'environnement de test, dans un vrai navigateur, un opérateur sans facteur se connecte,
scanne le QR (ou saisit la clé), voit sa session élevée et ses dix codes ; à la connexion suivante, un code de
l'application puis un code de récupération l'élèvent ; il remplace son application depuis `/`. axe ne relève
aucune violation, contraste compris.

## Arbitrages

Fable, 2026-10-20, sauf Z, tranché par l'humain le même jour.

110. **Défi en mémoire** : le défi de `login` vit dans une variable du module `session.tsx`, jamais dans le
     stockage du navigateur ; un rechargement le perd. `/mfa` ou `/enroll` sans défi renvoient à `/login`
     (avis « Reconnectez-vous »), comme `/` sur une session non élevée : un formulaire dont chaque envoi serait
     refusé est un cul-de-sac.
111. **Orientation** : après `login`, la SPA relit `me` : aucun facteur (`totp` faux, `passkeys` nul), `/enroll` ;
     sinon `/mfa` (89).
112. **`/mfa`** : un champ « Code à six chiffres » (`one-time-code`, `inputMode` numérique), et un bouton qui bascule
     vers « Code de récupération » et retour ; méthodes `totp` et `recovery_code`. Le refus est celui du serveur,
     un seul corps (99) ; le 429 suit `/login` (78). Le succès oublie `me` et ouvre `/`.
113. **`/enroll`** : un bouton « Configurer l'application » appelle `enrollTotp` ; pas d'appel au montage, que le
     double effet de StrictMode ferait remplacer. Puis le QR, la clé en clair et sa copie, le champ du code envoyé
     à `verifyMfa`, qui confirme et élève (84). Les dix codes n'apparaissent qu'après ce succès (d'abord prouver,
     ensuite garder), le focus sur leur titre ; « J'ai enregistré ces codes » ouvre `/`. Rien de l'enrôlement ne
     quitte l'état du composant (I2, 95).
114. **Remplacement** (humain, Z) : sur `/`, une section « Application d'authentification » dit les codes restants
     et offre « Remplacer » : preuve de l'ancien (code de l'application ou de récupération) envoyée à
     `enrollTotp`, puis le même composant d'enrôlement, confirmé par `confirmTotp` (85) ; ses nouveaux codes ;
     `me` relu. Les refus 409 et 422 rendent le message du serveur.
115. **QR** : `qrcode.react` 4.2.0, `QRCodeSVG`, 200 px, marge de 4 modules : un SVG sans style en ligne, que la
     CSP statique admet. `--qr-paper` revient avec lui (blanc, le contraste qu'une caméra exige).
116. **Copie** : un bouton « Copier » pour la clé et pour les codes, son issue annoncée (`aria-live`) ; un refus du
     presse-papiers le dit. Le téléchargement des codes de l'origine n'est pas porté : copier ou sélectionner
     suffit.
117. **Pas de bibliothèque de formulaire** : `FormData`, comme `/login` ; ni zod ni react-hook-form.

## Design arrêté

- **`web/package.json`** : `qrcode.react` 4.2.0.
- **`web/src/session.tsx`** : `rememberChallenge`, `challenge`, `forgetChallenge`.
- **`web/src/routes/login.tsx`** : garde le défi, relit `me`, oriente (111).
- **`web/src/routes/mfa.tsx`**, **`web/src/routes/enroll.tsx`** : les deux écrans.
- **`web/src/enrollment.tsx`** : `TotpEnrollment` (QR, clé, code, puis codes), servi par `/enroll` et `/` ;
  `CopyButton`.
- **`web/src/routes/index.tsx`** : garde d'élévation ; section du facteur et remplacement.
- **`web/src/styles/`** : `--qr-paper`, et les classes des écrans prises à l'`app.css` de l'origine (`.auth__qr`, `.auth__secret`,
  `.auth__codes`, `.auth__subtitle`, `.auth__aside`).
- **Tests** : écrans contre `fetch` simulé (corps envoyés, orientation, défi perdu, bascule de méthode, codes
  après succès seulement, focus, 429, remplacement et ses refus), axe sur chaque état ; contraste des nouvelles
  paires. Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium par `port-forward` : le parcours du « Terminé quand », le code TOTP
  calculé depuis la clé affichée ; captures ; axe dans le vrai navigateur ; aucune violation de CSP.
