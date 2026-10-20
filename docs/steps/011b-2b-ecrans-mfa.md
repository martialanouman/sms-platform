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

110. **Défi en mémoire** : le défi de `login` et son échéance vivent dans une variable du module `session.tsx`,
     jamais dans le stockage du navigateur ; un rechargement le perd, l'échéance le retire (118). `/mfa` ou `/enroll` sans défi renvoient à `/login`
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
     CSP statique admet. La bibliothèque peint elle-même le fond et la marge en blanc (attribut `fill`) : aucun
     jeton `--qr-paper`.
116. **Copie** : un bouton « Copier » pour la clé et pour les codes, son issue annoncée (`aria-live`) ; un refus du
     presse-papiers le dit. Le téléchargement des codes de l'origine n'est pas porté : copier ou sélectionner
     suffit.
117. **Pas de bibliothèque de formulaire** : `FormData`, comme `/login` ; ni zod ni react-hook-form.
118. **Échéance du défi** (revue, tour 1) : à `expiresAt`, `/mfa` et `/enroll` reviennent à `/login` (avis
     « vérification interrompue ») ; sans cela, un défi échu rend le même 401 qu'un code faux, sans fin. Le défi
     consommé par l'enrôlement est oublié aussitôt : l'échéance n'arrache pas les codes de l'écran.

## Design arrêté

- **`web/package.json`** : `qrcode.react` 4.2.0.
- **`web/src/session.tsx`** : `rememberChallenge`, `pendingChallenge`, `useChallengeExpiry`, `useElevated`,
  `meQuery`.
- **`web/src/ui.tsx`** : `useSubmission` (refus, focus, `aria-describedby`, `Retry-After`), servi par les cinq
  formulaires, `/access` compris.
- **`web/src/routes/login.tsx`** : garde le défi, relit `me`, oriente (111).
- **`web/src/routes/mfa.tsx`**, **`web/src/routes/enroll.tsx`** : les deux écrans.
- **`web/src/factor.tsx`** : `FactorProof` (code et bascule de méthode), `TotpEnrollment` (QR, clé, code, puis
  codes), `CopyButton`, `TotpReplacement` ; servis par `/mfa`, `/enroll` et `/`.
- **`web/src/routes/index.tsx`** : garde d'élévation ; section du facteur et remplacement.
- **`web/src/styles/app.css`** : les classes des écrans prises à l'`app.css` de l'origine (`.auth__qr`, `.auth__secret`,
  `.auth__codes`, `.auth__subtitle`, `.auth__aside`).
- **Tests** : écrans contre `fetch` simulé (corps envoyés, orientation, défi perdu, bascule de méthode, codes
  après succès seulement, focus, 429, remplacement et ses refus), axe sur chaque état ; contraste des nouvelles
  paires. Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium par `port-forward` : le parcours du « Terminé quand », le code TOTP
  calculé depuis la clé affichée ; captures ; axe dans le vrai navigateur ; aucune violation de CSP.

## Écarts au design, constatés à l'implémentation

- **`send` rend aussi `data`** : la connexion lit le défi, l'enrôlement son secret.
- **`/access` passe par `useSubmission`** : la même logique de refus y était écrite à la main.
- **« Annuler »** à l'étape de preuve du remplacement : la seule sortie sans recharger. Après l'enrôlement, il n'y en
  a pas : l'application en attente ne remplace rien tant qu'elle n'est pas confirmée (85), et un rechargement de `/`
  la laisse.
- **La garde de rendu `elevated` sur `/` est retirée** : la redirection suit le premier rendu, et ce qui s'affiche
  entre-temps appartient à l'opérateur.

## Mutations

Trente-sept, à la main, toutes tuées après deux ajouts de tests : le contenu du QR (comparé au rendu de l'URI),
le parcours complet de connexion (le cache de `me` d'avant l'élévation), la bascule de méthode qui vide le champ,
le compte sans application sur `/`. Liste : défi non gardé, orientation inversée, échec de relecture de `me`,
méthode figée, focus de bascule, `key` du champ, codes montrés avant la preuve, QR de la clé au lieu de l'URI,
issue de copie, focus des codes, séparateur des codes, Annuler, preuve du remplacement absente, `me` non relu après
remplacement, défi absent du corps, gardes de route, corps de l'enrôlement, oubli du défi et de `me` à
l'élévation, garde d'élévation de `/`, `Retry-After`, `aria-describedby` ; au tour 1, l'échéance (quatre façons),
l'effacement du refus, la ré-annonce de la copie, les focus d'intro et de retour, le niveau de titre, le défi non
gardé.

## Correctifs de revue, tour 1

Trois revues (correction et accessibilité ; tests creux ; code en trop). Aucun bloquant ; tous tranchés par Fable.

1. **Défi échu** (important) : arbitrage 118.
2. **Focus** (important, WCAG 2.4.3) : sur l'intro de l'enrôlement à son arrivée, sur le titre de la section au
   retour du remplacement.
3. **Mineurs corrigés** : refus effacé à la bascule de méthode ; copie ré-annoncée ; codes du remplacement sous un
   `h3` ; « Se connecter » désactivé jusqu'à la relecture de `me` (un second clic ouvrait une seconde session).
4. **Code en trop retiré** : déclarations CSS communes groupées ; un seul type `Call` ; le garde synchrone du
   presse-papiers (absent hors HTTPS et `localhost`, que le tableau de bord n'emploie pas) ; la purge de `me` avant
   sa relecture au login (`fetchQuery` relit une donnée périmée).
5. **Tests** : orientation d'un compte à clé d'accès seule ; boutons désactivés pendant leur appel ; 429 sur
   `/mfa` ; test de déconnexion ramené à ce qu'il prouve encore ; mocks restaurés entre les tests.
6. **Gardé** : le minuteur de `Retry-After` n'est pas annulé au démontage (un `setState` démonté est sans effet en
   React 19) et ne lit que des secondes (le BFF n'en envoie pas d'autres).

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.15.0 : image construite par GoReleaser en snapshot,
importée sous `proof-011b2b`, timer suspendu ; compte de test réinitialisé par un lien `reset` (aucun facteur). Dans
Chromium (`agent-browser`), par tunnel SSH vers `port-forward`, sur `localhost:8080`, chaque code TOTP calculé
depuis la clé affichée. Clé, QR et codes sont floutés sur les captures ; un second `reset`, en fin de preuve, les a
annulés (aucun facteur au compte).

1. Connexion sans facteur : `/enroll` ([capture](011b-2b/enrolement.png)) ; « Configurer l'application », focus sur
   l'intro, QR SVG de 200 px, deux chemins, fond `#FFFFFF` ([capture](011b-2b/qr.png)) ; le code élève la session,
   focus sur « Codes de récupération », dix codes ([capture](011b-2b/codes.png)) ; `/` dit « 10 codes de
   récupération restants. » ([capture](011b-2b/accueil.png)).
2. Connexion suivante : `/mfa`, focus sur le champ ([capture](011b-2b/verification.png)) ; un code faux, refus
   focalisé ([capture](011b-2b/refus.png)) ; le code de l'application ouvre `/`.
3. Connexion suivante, code de récupération ([capture](011b-2b/recuperation.png)) : `/` dit « 9 codes ».
4. Remplacement depuis `/`, prouvé par l'application : nouvelle clé, distincte ([capture](011b-2b/remplacement.png)) ;
   son code confirme, titre des codes en `h3` ; retour, focus sur « Application d'authentification », « 10 codes ».
5. Connexion suivante : un code de l'ancienne clé est refusé, un code de la nouvelle élève.
6. axe dans la page, `color-contrast` comprise : aucune violation sur les dix états ; quatre-vingt-dix-neuf
   requêtes, toutes vers l'origine ; aucune violation de CSP ni erreur de console.
