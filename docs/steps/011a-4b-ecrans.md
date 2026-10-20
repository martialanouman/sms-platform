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
81. **Repli** (correctif de revue) : une réponse se juge sur son statut (`response.ok`). Une erreur réseau, ou un refus
    sans corps du contrat (un 502 du proxy), affiche un texte de repli rédigé par la SPA, « Le service ne
    répond pas ; réessayez. », et libère le bouton ; elle ne passe jamais pour un succès. Le client de production ne
    relance pas un 401 sur `me`.

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

## Écarts au design, constatés à l'implémentation

- **Pas de refus local des champs vides** : le serveur juge et rédige (arbitrage 69) ; un formulaire vide part, et le
  422 du contrat s'affiche comme tout refus. La mutation du contrôle local survivait, faute de raison d'être.
- **Le harnais nettoie** entre deux tests (rendu, `fetch` doublé, minuteurs) : Vitest sans `globals` ne déclenche pas le
  nettoyage automatique de Testing Library.

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.13.0, dans Chromium piloté par `agent-browser` (l'extension
Chrome n'était pas joignable) : image construite par GoReleaser en snapshot, importée sous `proof-011a4b`, puis
`proof-011a4b-2` après correctif ; timer de déploiement suspendu, puis réarmé et image de la release rétablie. Tunnel
SSH vers `port-forward`, sur `localhost:8080`, l'origine déclarée du dashboard. Un lien de réinitialisation est mis en
file par SQL sous `app.actor` (`QueueLink`), l'opérateur de bootstrap ayant déjà son mot de passe.

1. Le lien arrive dans Mailpit ; `/access#…` s'ouvre, le fragment quitte l'adresse, titre « Mot de passe — Tableau de
   bord ».
2. Un mot de passe faible : 422, message du serveur en alerte focalisée, liée au champ par `aria-describedby`.
3. Un mot de passe valide : `/login?avis=mot-de-passe-enregistre`, avis affiché, focus sur l'adresse.
4. Connexion : `/`, « Connecté en tant que … », titre « Tableau de bord ».
5. Déconnexion : `/login`, sans avis ; `/` rouvert ensuite renvoie à `/login?avis=session-fermee`.
6. Session antidatée de 15 min, un geste : `/login?avis=session-fermee`.

Rouge vu : à la première image, la déconnexion menait à `/login?avis=session-fermee`. Le `pointerdown` du clic envoie
une activité, à laquelle le serveur, la session déjà fermée, répond 401 ; ce retour poussait l'avis après `/login`. Le
journal des requêtes l'a montré (une hypothèse sur `removeQueries`, testée d'abord, ne reproduisait rien). Correctif :
la déconnexion retire l'activité avant d'envoyer `logout`, et une réponse d'activité n'agit plus une fois son effet
démonté ; le test sert les deux réponses dans le même tour, et chaque garde tombe sous mutation.

## Mutations

Vingt-quatre mutations manuelles (constantes, en-têtes, focus, titres, avis, fragment, minuteur, 429, gardes de
déconnexion) : toutes tombent, sauf le refus local des champs vides, retiré.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant ; 28 constats.

1. **Arbitrage 81** : succès sur `response.ok`, repli sur une erreur réseau ou un corps hors contrat, par `send` dans
   `api.ts`, seule voie des trois écrans et de l'activité. Un 502 sans corps menait à « Mot de passe enregistré » ; une
   erreur réseau bloquait le bouton à vie.
2. **`me` en 401 n'est plus relancé** par le client de production (trois relances, 7 s d'écran vide) : le `retry` vit
   dans `main.tsx`, et un test monte la SPA réelle, perdu depuis 011a-4a.
3. **Un avis inconnu n'est plus affiché** : le routeur fusionne la recherche brute avec la recherche validée ; la
   validation rend `avis: undefined`, par `Object.hasOwn`.
4. **Deux refus identiques reprennent le focus** : l'erreur se vide avant chaque envoi.
5. **Une déconnexion qui échoue** le dit, et rend la session à l'activité et au minuteur.
6. **`me` en échec hors 401** affiche le repli.
7. **Retraits** : le texte des règles du mot de passe (arbitrages 69 et 76 : le 422 du serveur le dit) ; `removeQueries`
   à la fermeture de session et à la déconnexion, sans effet (celui du login reste : il empêche d'afficher l'opérateur
   précédent) ; la ref d'activité ; les exports `ApiError` et `mount`.
8. **Tests** : le doublé enregistre l'appel inattendu avant de lever ; 429 sous fausse horloge, borné des deux côtés ;
   historique remplacé ; focus et `aria-describedby` sur `/access` ; minuteur absent si `null` ; bornes de temps
   élargies ; signal au lieu d'un délai réel ; axe sur chaque état.
9. **Gardés tels quels** : `required` sous `noValidate` (annoncé aux lecteurs d'écran) ; la mécanique commune de login et
   d'access, deux usages (011b en fera un troisième) ; `renderAt` hors de `newRouter`.

Mutations du tour : 31, toutes tombent après comblement (le refus répété sur `/access`, `removeQueries` du login,
`setLeaving(false)`) ; `disabled` posé sur la déconnexion survivait, retiré.

Preuve rejouée après le tour, sur `proof-011a4b-3` : lien, refus 422 focalisé, mot de passe, connexion, déconnexion sur
`/login` sans avis, `/` rouvert renvoyé à l'avis, session antidatée renvoyée à l'avis ; `/login?avis=toString` n'affiche
aucun avis. Les connexions répétées de la preuve ont déclenché l'anti-force brute : 429, bouton désactivé le temps de
`Retry-After`, puis réactivé, dans le navigateur.
