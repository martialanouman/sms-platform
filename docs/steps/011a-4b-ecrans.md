# Step 011a-4b — Écrans de connexion et de lien d'accès

Plan : `docs/plan/composants.md` §011, seconde des deux PR de 011a-4. Arbitrages : `docs/steps/011a-4a-chaine-spa.md`
(64 à 73, dont 68, 69 et 73 pour les écrans). Spec : `docs/spec/08-dashboard.md` §8.3, §8.8 ; contrat
`contracts/openapi-bff.yaml` (`login`, `setPasswordFromAccessLink`, `me`, `reportActivity`, `logout`). Règles :
`.claude/rules/tests.md`. Mode : PR.

## Contexte

- La SPA est embarquée et servie (011a-4a) ; elle n'a qu'un squelette. Le BFF sert les sept opérations (011a-3b).
- Dettes du territoire : #9 (CSP par requête, assets inter-versions), reportée à 026, inchangée.

**Terminé quand** : dans un vrai navigateur, l'opérateur de bootstrap ouvre son lien, pose son mot de passe, se
connecte, voit son identité, se déconnecte ; sans geste pendant 15 min, il revient à la connexion avec un avis ; chaque
écran passe axe sans violation.

## Arbitrages propres à 011a-4b

Tranchés par l'agent, sans contredire 1 à 73.

74. **Un client unique** (`src/api.ts`), `openapi-fetch` sur `/api` avec les types générés. Les requêtes sans corps
    (`logout`, `activity`) déclarent `Content-Type: application/json` ; le navigateur pose `Origin` de lui-même.
75. **Une session** (`src/session.tsx`) : la requête `me` sous TanStack Query ; un 401 sur toute requête protégée vide la
    requête et renvoie à `/login?avis=session-fermee`. Activité : `pointerdown` et `keydown` sur `document`, un
    `POST /auth/activity` au plus toutes les 60 s ; le même geste réarme un minuteur d'`idleTimeoutSeconds` (aucun si
    `null`), qui renvoie à la connexion sans requête.
76. **Avis par paramètre de recherche** (`avis` : `session-fermee`, `mot-de-passe-enregistre`), validé par la route ; les
    textes vivent dans la SPA, seuls ceux-là (arbitrage 69).
77. **Le jeton du lien** se lit dans le fragment au premier rendu, puis `history.replaceState` le retire de l'adresse. Sans
    jeton, l'écran dit que le lien est incomplet et n'envoie rien.
78. **429** : le message du serveur s'affiche, le bouton reste désactivé `Retry-After` secondes, puis se réactive.
79. **Accessibilité** : un `<h1>` et un titre de document par écran ; libellés explicites ; erreurs en `role="alert"`
    liées au champ par `aria-describedby` ; focus sur le premier champ à l'ouverture, sur l'alerte après un refus ;
    `autocomplete` (`username`, `current-password`, `new-password`).
80. **Développement** : le BFF local tourne avec `DASHBOARD_PUBLIC_URL=http://localhost:3000`, l'origine du serveur
    Vite, sans quoi `guard` refuse les requêtes (011a-4a, tour 1).

## Design arrêté

- **`web/src/api.ts`** : le client.
- **`web/src/session.tsx`** : `useMe`, `useActivity`, la redirection sur 401 et à l'inactivité.
- **`web/src/routes/login.tsx`**, **`access.tsx`**, **`index.tsx`** : les trois écrans ; `__root.tsx` porte le cadre.
- **`web/src/test.tsx`** : rendu d'une route sous ses providers, `fetch` doublé qui refuse tout appel non déclaré,
  helper axe (règle `color-contrast` désactivée, que jsdom ne calcule pas).
- **Tests**, un fichier par écran :
  - connexion : succès, 401, 429 et son délai, 503, avis affichés ;
  - lien : jeton retiré de l'adresse, 204, 410, 422 de politique, jeton absent ;
  - accueil : identité affichée, 401, déconnexion qui part avec `Content-Type`, activité limitée à une requête par
    minute, minuteur réarmé par un geste et qui renvoie à la connexion ;
  - axe sans violation par écran.

  Mutations manuelles, consignées.
- **Preuve sur `contabo169`** dans Chrome, par `port-forward` : lien reçu dans Mailpit, mot de passe, connexion,
  accueil, déconnexion ; session antidatée de 15 min, retour à la connexion avec l'avis.
