# Step 013d — Écran Audit

Plan : `docs/plan/composants.md` §013, dernière des quatre PR de 013 (coupe : arbitrage 229). Arbitrages antérieurs : 1 à
258 (`docs/steps/013a-*.md` à `013c-*.md`, `012*.md` pour la SPA). Spec : `docs/spec/08-dashboard.md` §8.1, §8.8 ;
design 05 §4.6. Contrats : `contracts/openapi-admin.yaml` (`list-audit-log`, `list-audit-changes`), `contracts/openapi-bff.yaml`
(`listAccessLog`, `issueGatewayToken`). Issue : #165. Règles : `.claude/rules/tests.md`, `contracts.md`. Mode : PR.

## Contexte

- Le proxy `/api/gw/*` relaie l'Admin API (013c) ; la SPA n'a pas de types du contrat Admin.
- `GET /access-log` est servi par le BFF (012a) sans écran.
- « Mon compte » délivre un jeton personnel portant tous les scopes permis, sans choix ; le bouton « Copier le secret »
  est trop bas (#165, retour humain de la preuve de 013b).

**Terminé quand**, navigateur sur `contabo169`, session élevée de `Propriétaire` : « Audit » au rail ; l'onglet
« Plan de contrôle » montre les lignes de la passerelle, le filtre par opérateur la réduit, « Suivant » charge la page
suivante, « Modifications » ouvre l'avant/après d'une requête ; l'onglet « Accès au dashboard » montre la connexion du
moment. Rôle `Reporting` seul : « Audit » absent du rail, `/audit` dit le refus. `admin-api-svc` arrêté : « Plan de
contrôle » en erreur avec « Réessayer », « Accès au dashboard » intact. « Mon compte » : un jeton délivré avec deux
scopes cochés en porte deux, et « Copier le secret » tient dans la modale.

## Arbitrages

Fable, 2026-10-22, sauf mention.

259. **Pas de coupe.** **Types Admin** : `openapi-typescript` sur `contracts/openapi-admin.yaml` vers `web/src/admin.gen.ts`
     (une ligne de `go:generate`, couverte par `check-generated`). Client `web/src/gw.ts` sur `/api/gw` ; l'intercepteur
     `mfa_required` de `api.ts` sert aux deux clients ; `send` et `read` servent tels quels.
260. **Prose de `AuditEntry.target`** (décision humaine de 013a) : la mention du masquage par `msisdn:reveal` est retirée
     du contrat Admin, prose seule ; `target` est le chemin sans requête (233), et aucun chemin ne porte de numéro.
261. **Route** : `/audit`, groupe « Administration » du rail, permission `audit:read`. Sans elle, l'URL directe rend
     l'écran avec son refus expliqué, requêtes désactivées, comme `/operators`.
262. **Onglets** : deux routes filles (`/audit/control-plane`, `/audit/access`), `/audit` mène à la première ; des liens
     avec `aria-current="page"`, l'URL portant l'onglet, les filtres et le curseur. Deux requêtes, deux tableaux, jamais
     fusionnés (§8.8). Classes d'onglets neuves, en jetons de la charte.
263. **Plan de contrôle** : colonnes Date, Opérateur, Opération, Cible, Statut (et `outcome` non nul) ; filtres du contrat
     seuls (`operator` exact, `from_date`, `to_date` en `datetime-local` convertis en ISO) ; « Suivant » sur
     `next_cursor`, « Précédent » par l'historique du navigateur ; `limit` du contrat (50). « Modifications » sur une
     ligne à `request_id` : modale, `list-audit-changes`, par changement la table, la clé, l'opération, l'acteur, puis
     l'union des colonnes de `before` et `after`, une colonne changée marquée ; vide → « Aucune modification
     enregistrée ». Aucune bibliothèque de diff.
264. **Nom de l'opérateur** : `operator` est l'identifiant de l'opérateur ; affiché brut, et son nom au-dessus si
     `operators:read` est détenu (requête existante) ; `service:` et `breakglass:` restent bruts. Même aide pour `actor`
     et `target` de l'onglet d'accès.
265. **Accès au dashboard** : `GET /access-log`, colonnes Date, Acteur, Événement, Cible, Résultat ; libellés français de
     chaque événement du contrat en table exhaustive (`tsc` refuse un événement sans libellé).
266. **Erreurs** : un 4xx de la passerelle (`forbidden_scope`) rend son message ; `mfa_required` mène à `/mfa` ; 401 ferme
     la session ; 503 → « Le service ne répond pas », « Réessayer ».
267. **I1 et cache** : aucune donnée d'audit ne porte un corps ; requêtes en mémoire seulement (vidée à la déconnexion),
     rien de persisté.
268. **#165, scopes** (décision humaine de 013b) : la génération écrit aussi `AdminScope` ; le sélecteur par ressource des
     rôles devient un composant partagé ; catalogue du jeton = scopes Admin détenus moins `content:read`,
     `content:erase`, `gdpr:erase`, tout coché par défaut, `scopes` toujours envoyé ; aucun coché → « Délivrer »
     désactivé et expliqué.
269. **#165, bouton** : « Copier le secret » passe au pied de la modale, à côté de « J'ai copié le secret », en `Button`
     du kit.
270. **Accessibilité** : `aria-current` des onglets, `caption` des tableaux, filtres étiquetés, focus au titre, `axe` dans
     chaque test.
271. **Dettes** : #169 (`go-live`, décision humaine) — le diff d'audit rendrait un MSISDN en clair ; ouverte, sans effet
     en 013d (aucune table à numéro). #165 soldée.

Retours humains de la preuve navigateur, 2026-10-22 :

272. **Recherche par nom ou e-mail** : reportée (humain), issue #170 ; le filtre reste l'identifiant exact.
273. **Dates sans heure** : « Depuis » et « Jusqu'à » sont des jours ; « Depuis » vaut le début du jour local, « Jusqu'à »
     sa fin, envoyée comme le début du lendemain (borne exclusive des contrats). Par défaut, le jour courant pour les deux
     bornes. L'URL porte les jours (`AAAA-MM-JJ`).
274. **Bouton « Filtrer »** à la hauteur des champs, aligné sur eux, libellé exclu.
275. **Journal d'accès filtrable** : `listAccessLog` gagne `actor` (identifiant exact), `fromDate` (incluse) et `toDate`
     (exclue) au contrat BFF, d'abord ; le port, la requête SQL et le handler suivent ; l'onglet reprend les champs et le
     défaut de 273.

Second retour, même jour :

276. **Paramètre mal formé** : un `actor` qui n'est pas un UUID passait le validateur (kin-openapi n'impose pas le format
     `uuid`) et tombait au décodage généré, qui répondait `400 text/plain` hors contrat ; le SPA le prenait pour une
     panne. Le BFF répond désormais `422 validation_error`, le paramètre placé.
277. **Opérateur choisi dans une liste** (humain ; source arbitrée par Fable) : annuaire `GET /api/audit/operators` sous
     `audit:read` (`id`, `email`, `displayName`, `status`, actifs et désactivés), pas `/operators` élargi, qui livrerait
     rôles, second facteur et lien d'accès à un rôle `Audit` qui n'a que `audit:read`. Il nomme aussi les opérateurs des
     deux tables, sans `operators:read`. Le champ est un `<input list>` + `<datalist>` natif filtrable, valeur l'e-mail
     (unique), résolu en identifiant à la soumission ; un texte inconnu part tel quel (`service:<nom>`,
     `breakglass:<nom>` du journal de la passerelle). Les identifiants sont les mêmes des deux côtés : `sub` du jeton
     émis par le BFF = identifiant de l'opérateur. Plafond : Safari ne filtre que par préfixe ; combobox ARIA si un
     auditeur y bute. Solde #170, qui remplace 272.
278. **Kit sur Base UI** (humain ; révise 102 et 184) : la liste native n'obéit pas à la charte (le système la peint) ;
     `@base-ui/react` 1.8.0 (1.9.0 sous l'âge minimal de pnpm), la bibliothèque de l'ancienne SPA, devient la base de
     tout composant du kit qui a un équivalent, dans cette PR (humain) : `Field`, `Button`, `Modal` et `Confirmation`
     (`Dialog`, `AlertDialog`), les cases du sélecteur de permissions (`Checkbox`), et le filtre « Opérateur », un
     `Combobox` (remplace le `<datalist>` de 277 : valeur l'identifiant, filtre sur le nom et l'e-mail). Sans
     équivalent, `DataTable`, `EmptyState`, `LoadingState`, `Toolbar`, `PageHeader`, `Icon` et les onglets d'Audit
     (des liens de route) restent du balisage. Un composant neuf naît du besoin d'un écran (humain). Le texte libre de
     277 disparaît : le filtre ne choisit que dans l'annuaire.

## Design arrêté

- **`contracts/openapi-admin.yaml`** : prose de `AuditEntry.target` (260).
- **`doc.go`**, **`web/src/admin.gen.ts`**, **`tools/openapi`** (`AdminScope`), **`web/src/scopes.gen.ts`**.
- **`web/src/gw.ts`** ; **`web/src/api.ts`** (intercepteur partagé).
- **`web/package.json`** (`@base-ui/react`, 278) ; **`web/src/ui.tsx`** et **`web/src/picker.tsx`** (278).
- **`web/src/routes/_shell.audit*.tsx`** ; **`web/src/routes/_shell.tsx`** (rail) ; **`web/src/routes/_shell.account.tsx`**
  (#165) ; **`web/src/routes/_shell.roles.tsx`** (sélecteur sorti) ; **`web/src/styles/app.css`**.
- **`contracts/openapi-bff.yaml`** (275, 277) ; **`internal/dashboard/http`** (276, 277) ; **`internal/dashboard`** (port, filtre du journal),
  **`internal/dashboard/postgres`** (requête), **`internal/dashboard/http`** (handler).
- **Tests** Vitest par écran (cas de 261 à 275) ; test Go de `tools/openapi` pour `AdminScope` ; journal d'accès filtré
  contre Postgres réel et par HTTP. Mutation manuelle.
