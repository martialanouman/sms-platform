# Step 011a-4a — SPA : chaîne, embarquement, en-têtes

Plan : `docs/plan/composants.md` §011, première des deux PR de 011a-4 (humain, 2026-10-20, sur recommandation de
Fable). Arbitrages antérieurs : 1 à 63 (`011a-1` à `011a-3b`), dont 12 (la SPA, sa chaîne, son embarquement). Spec :
`docs/spec/08-dashboard.md` §8.1, §8.3, §8.8 ; `docs/spec/01-exigences.md`. Contrat : `contracts/openapi-bff.yaml`.
Origine : `go-gateway-bo/web`. Règles : `.claude/rules/ci-deploy.md`, `.claude/rules/tests.md`. Mode : PR.

Ce document porte les arbitrages de **toute la 011a-4** (64 à 73) ; 011a-4b y renvoie.

## Contexte

- Le dashboard sert `/api` (011a-3b) ; `/` n'est pas monté. Aucun code TypeScript, aucun Node en CI.
- `.gitignore` porte `dist/` sans ancre.
- Dettes du territoire : #20 (binaire sans Node), soldée ici ; #9 (CSP par requête, assets inter-versions), reportée
  à 026 ; #5, #6, #21, sans objet (arbitrage 72).

**Terminé quand** : la CI construit la SPA avant les tests Go et la release l'embarque ; sur l'environnement de test,
`GET /` rend `index.html` avec ses en-têtes de sécurité, un asset haché est servi en cache immuable, `/api` est
inchangé.

## Arbitrages

Fable, 2026-10-20, sauf M à P, tranchés par l'humain le même jour. Le routeur (P) suit l'humain contre la
recommandation de Fable.

64. **Coupe** (humain, M) : 011a-4a, chaîne, embarquement, CI, en-têtes, #20, preuve `GET /` ; 011a-4b, les trois écrans,
    l'activité, la fin de session, les tests d'accessibilité, la preuve du parcours dans Chrome.
65. **Pile** : `react` et `react-dom`, `@tanstack/react-router` et son plugin Vite, `@tanstack/react-query` (spec 08
    §8.1), `openapi-fetch` et `openapi-typescript`, `vite` et `@vitejs/plugin-react`, `typescript`, `@biomejs/biome`,
    `vitest`, `jsdom`, `@testing-library/react` et `user-event`, `axe-core`. Ni formulaires typés ni zod : deux
    formulaires de deux champs, le serveur juge. Projet sous `web/`, `pnpm-lock.yaml` commité, `packageManager` et
    `engines` déclarés ; `pnpm-workspace.yaml` porte `minimumReleaseAge: 20160` (deux semaines), qui rend la règle des
    versions mécanique. Dependabot suit `/web`.
66. **Types générés** : `openapi-typescript` sous `go generate`, généré `web/src/api.gen.ts` commité, couvert par
    `check-generated`.
67. **Routeur fichier** (humain, P) : `@tanstack/router-plugin`, routes sous `web/src/routes/`, arbre
    `web/src/routeTree.gen.ts` commité et régénéré par `go generate` (`tsr generate`), donc couvert par
    `check-generated` ; les tests ne deviennent pas des routes.
68. **Écrans** (011a-4b) : un seul écran `/access`, le contrat ne donnant au client que le jeton ; le jeton se lit dans le
    fragment, puis le fragment s'efface. `/login`, puis `/` (identité, déconnexion) ; 011b insérera le second facteur.
    Activité : `pointerdown` et `keydown`, au plus un `POST /auth/activity` par minute. Fin de session (humain, O) : un
    minuteur local réarmé à chaque geste renvoie à `/login` au bout d'`idleTimeoutSeconds`, sans requête ; un 401 sur
    toute requête protégée en fait autant.
69. **Messages** : le serveur rédige ; la SPA affiche `message` et n'aiguille que sur `code`.
70. **Embarquement** : paquet Go `web` (`//go:embed all:dist`), `web/dist/.gitkeep` commité, `.gitignore` ancré ;
    `make web` construit ; `http.Config.Assets` ; le mux racine sert `/api/` par la chaîne actuelle, et `/` en GET et
    HEAD : un asset existant sous `assets/` en cache immuable, tout le reste `index.html` en `no-cache`. En-têtes sur
    toute réponse : `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`, et (humain, N) une CSP statique
    `default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`. Le nonce par requête et les
    assets inter-versions restent #9.
71. **CI** : pnpm et Node, épinglés par SHA, dans `check` et `release` ; `make web` avant les tests Go ; `make lint`
    ajoute Biome et `tsc` ; `make vuln` ajoute `pnpm audit` ; CodeQL couvre `javascript-typescript`. GoReleaser joue
    `make web` en `before.hooks`.
72. **Tests** : Vitest, Testing Library, `axe-core` par un petit helper ; mutation manuelle, comme en Go. #20 soldée
    par un test de `cmd/dashboard` qui exige `index.html` embarqué ; #5, #6 et #21, sans objet (la charte de design de
    l'origine n'est pas portée), fermées par la PR qui le constate.
73. **À ne pas oublier** (011a-4b) : `logout` et `activity` envoient `Origin` et `Content-Type: application/json` sans
    corps ; `Retry-After` se lit dans l'en-tête ; `lang="fr"` et un titre ; focus sur le premier champ ou sur l'alerte
    ; jamais de mot de passe ni de jeton dans une URL ou un journal (`noConsole` en erreur).

## Design arrêté

- **`web/`** : `package.json` (dépendances de l'arbitrage 65, scripts `build`, `test`, `lint`, `typecheck`),
  `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `tsconfig.json`, `biome.json`, `vite.config.ts` (plugin du routeur, proxy
  `/api` en dev), `index.html` (`lang="fr"`), `src/main.tsx`, `src/routes/__root.tsx` et `src/routes/index.tsx`
  (squelette : un titre), `src/routeTree.gen.ts`, `src/api.gen.ts`, un test Vitest du squelette ; `web/web.go`
  (paquet Go, `FS()`), `web/dist/.gitkeep`.
- **`doc.go`** : génération des types et de l'arbre de routes.
- **`internal/dashboard/http`** : `Config.Assets` ; mux racine ; service des assets et repli ; en-têtes.
- **`cmd/dashboard`** : passe `web.FS()`.
- **`Makefile`** : `web` ; `test` et `ciguard` en dépendent ; `lint`, `vuln` étendus. **`.goreleaser.yaml`** :
  `before.hooks`. **`.github/workflows/ci.yml`** : pnpm et Node dans `check` et `release`, CodeQL JS.
  **`.github/dependabot.yml`** : `/web`. **`.gitignore`** ancré.
- **Tests** : handler sur `fstest.MapFS` (repli, immuable, `no-cache`, HEAD, méthode refusée, en-têtes, `/api`
  inchangé) ; `cmd/dashboard` exige `index.html` embarqué ; Vitest sur le squelette ; garde de livraison inchangée.
- **Preuve sur `contabo169`** : `GET /` 200 avec ses en-têtes, un asset haché immuable, une route inconnue rend
  `index.html`, `/api/health` inchangé.

## Écart au design, constaté à l'implémentation

`openapi-typescript` 7.13 appelle l'API JavaScript de TypeScript, que TypeScript 7 (natif) ne fournit plus, et déclare
`typescript ^5.x` en pair : le projet suit TypeScript 5.9.3. Le formateur de Biome est réglé sur le style écrit
(espaces, guillemets simples, sans point-virgule).

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.12.0 : image construite par GoReleaser en snapshot, dont
le hook `make web` a construit la SPA ; importée sous le tag `proof-011a4a` ; timer de déploiement suspendu pendant la
preuve, puis réarmé. Par `port-forward` :

1. `GET /` : 200, `index.html` (`<html lang="fr">`, titre « Tableau de bord »), `Cache-Control: no-cache`, `nosniff`,
   `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`, CSP statique.
2. `GET /assets/index-….js` : 200, `public, max-age=31536000, immutable`, `text/javascript`.
3. `GET /access/x` : 200, `index.html`.
4. `GET /api/health` : 200 inchangé ; `POST /` : 405.

Rouge vu : un premier import échouait en silence (`k3s` hors du `PATH` de la session), et le tag `proof` désignait
encore l'image de 011a-3b. Le pod servait donc l'ancien 404 JSON sur `/`. Rejoué avec le chemin explicite et un tag
neuf.

## Correctifs de revue, tour 1

Deux revues (correction et sécurité ; tests creux et code en trop). Aucun bloquant ; 16 constats.

1. **Un asset absent rend 404** (révise l'arbitrage 70) : sous `assets/`, un fichier absent ou un répertoire rend 404,
   jamais `index.html`. Un onglet ouvert sur une version retirée voyait du HTML refusé par `nosniff`, sans diagnostic
   possible.
2. **Aucun `data:` dans le build** : `assetsInlineLimit: 0`, pour que la CSP `default-src 'self'` ne bloque pas en
   silence une petite image ou une police inlinée.
3. **Le build garde `.gitkeep`** : le script `build` le recrée, `vite build` vidant `dist`, et `make web` n'a plus à
   le faire.
4. **Dependabot npm attend 14 jours** (`cooldown`) : `minimumReleaseAge` ne joue qu'à la résolution, jamais sous
   `--frozen-lockfile` ni pour Dependabot.
5. **Proxy de dev** limité à `/api/` ; en dev, le BFF tourne avec `DASHBOARD_PUBLIC_URL=http://localhost:3000`, faute de
   quoi `guard` refuse l'origine du serveur Vite (pour 011a-4b).
6. **Tests resserrés** : `Allow` sur le 405 ; un sous-répertoire d'`assets/` ; le montage réel de `main.tsx` ; le test
   du squelette déplacé sous `routes/`, où le motif d'exclusion du routeur devient une propriété prouvée.
7. **Retraits** : la configuration du routeur doublée dans `vite.config.ts` (elle vit dans `tsr.config.json`, que lisent
   la CLI et le plugin) et ses valeurs par défaut, `outDir`, le `--noEmit` doublé, le `FileServerFS` remplacé par
   `ServeFileFS`.
8. **Gardés tels quels** :
   - `pnpm audit` échoue sur tout avis, devDependencies comprises : le libellé de `vuln` le dit désormais.
   - `index.html` est embarqué sans date, donc sans validateur : retransféré à chaque navigation, pour quelques
     centaines d'octets.
