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
279. **« Précédent »** (humain) : les curseurs des deux journaux ne vont que vers l'avant ; l'URL garde la pile des
     curseurs parcourus (`trail`, `-` pour la première page), « Suivant » l'empile, « Précédent » la dépile et
     n'apparaît que si elle n'est pas vide. Filtrer la vide.
280. **Nom de l'onglet** (humain, révise la spec 08 l.109) : « Plan de contrôle » devient « Administration de la
     passerelle » ; « Accès passerelle », proposé, se confondrait avec les accès des clients SMS (SMPP, REST). Le
     chemin `/audit/control-plane` reste, identifiant de code.
281. **Écran refusé** (humain, sur proposition ; guide des messages d'erreur de Google, états vides de Material) : la
     phrase brute « Il faut la permission … » devient un `Forbidden` du kit, sur l'état vide de la charte : titre
     (« Accès aux … non accordé »), ce que l'écran contient, la permission par son libellé français et sa clé en mono,
     qu'aucun rôle du compte n'accorde, et l'issue (le rôle par défaut qui l'accorde, ajouté par un opérateur habilité).
     Pas de bouton : l'opérateur n'a rien à faire lui-même. Audit, Opérateurs et Rôles. Solde en partie #160 (les
     bulles des boutons bloqués restent en prose).

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

## Écarts au design, constatés à l'implémentation

- **Validation des champs** : `Field` passe sur les parties `Field` de Base UI, mais garde la validation native du
  formulaire (`onInvalid`, messages en français, focus au premier champ invalide) ; celle de Base UI suppose son
  `Form`, que les écrans n'emploient pas.
- **Focus rendu au démontage** : la modale est montée par l'écran tant qu'elle est ouverte, et démontée aussitôt fermée,
  sans transition ; `finalFocus` n'y joue pas, le focus revient à l'ouvreur au démontage, comme avant.
- **Bulle d'un bouton bloqué** : ouverte sans délai et gardée au clic (`closeOnClick` faux) ; Échap la ferme d'abord, la
  modale ensuite.
- **`--anchor-width`** : seule variable CSS hors des tokens, posée par les positionneurs de Base UI ; le test des tokens
  la nomme.
- **Biome** : `noLabelWithoutControl` reconnaît `CheckboxRoot`, alias de `Checkbox.Root` (la règle refuse un nom
  pointé).
- **Simulation de `<dialog>`** retirée des tests : plus aucun `<dialog>` natif.
- **Le filtre par défaut du `Combobox`** compare par `Intl.Collator`, qui ignore la ponctuation : `matches` filtre sur le
  nom et l'e-mail, et le test tape une part d'e-mail absente du nom.

## Mutations

À la main, par le script de 013a, puis par un script de copie, mutation et restauration (`cmp`) sur la SPA. Les
premières unités (types, écran, #165, filtres par jour, journal d'accès filtré) ont été mutées à l'écriture, sans
compte consigné. Ensuite : paramètre mal formé en 422 2/2 ; annuaire `audit/operators` 2/2 (statut, scope du contrat) ;
filtre par liste (`<datalist>`) 5/5 après un test creux remplacé ; `Combobox` 8/8 (un test creux renforcé) ; `Modal`,
`Button` et sa bulle 5/5 ; `AlertDialog` 1/1 ; `Field` 2/2 ; cases 4/4 ; « Précédent » 6/6 ; `Forbidden` 5/5. Le
survol des onglets (CSS) n'a pas de test : jsdom n'applique pas la cascade, la preuve navigateur le couvre.

## Preuve sur `contabo169`

Jouée le 2026-10-22 depuis la branche, sur le cluster en v0.27.0 : images `dashboard` construites localement,
importées sous `proof-013d-2` à `proof-013d-7` au fil des retours ; timer suspendu, puis réarmé, qui ramène la
release.

1. **Navigateur, humain** : les deux onglets, filtres par jour au jour courant, opérateur choisi dans la liste par nom
   ou e-mail, « Suivant » et « Précédent », « Modifications » et son diff, jeton personnel par scopes et copie au
   pied, modales, confirmations, cases et bulles sur Base UI, sans erreur de CSP. Retours 272 à 281 traités sur place.
2. **Rôle `Reporting` seul** : « Audit » absent du rail ; `/audit`, `/operators` et `/roles` montrent l'écran refusé de
   281, sans appel aux journaux. Un redéploiement pendant l'essai a rendu `Propriétaire` à l'opérateur d'amorçage
   (`Bootstrap`, voulu ; `roles_changed` sans acteur au journal d'accès) ; le rôle a été réduit à nouveau.
3. **`admin-api-svc` à zéro réplique** : l'onglet « Administration de la passerelle » dit l'échec avec « Réessayer » ;
   l'onglet « Accès au dashboard » et l'annuaire restent servis. Réplique rendue, rôle `Propriétaire` rendu.
4. **Sans session** : `GET /api/access-log?actor=x` 422 `validation_error` placé sur `actor` ; `GET /api/audit/operators`
   401.

## Correctifs de revue, tour 1

Trois revues en lecture seule : correction et sécurité, tests creux, code en trop.

1. **Filtres affichés figés** (moyenne) : le formulaire, non contrôlé, gardait l'opérateur et les jours d'un filtrage
   précédent quand l'adresse changeait sans lui (onglet rouvert, Retour) ; il se remonte sur l'opérateur et les jours
   de l'adresse.
2. **Modifications tronquées en silence** (moyenne) : la liste des changements d'une requête est paginée par la
   passerelle (50) ; la modale porte « Suivant » et « Précédent », comme les journaux.
3. **Jour invalide dans l'adresse** (basse) : `2026-13-01` cassait l'écran, `2026-02-30` glissait au 2 mars ; un jour
   qui ne se relit pas à l'identique retombe sur aujourd'hui.
4. **Clés en double dans la modale** (basse) : deux changements d'une même ligne dans une requête ; la clé porte l'index.
5. **Tests creux** : session fermée sur 401 (deux journaux et modifications), statut et `outcome`, cible nommée,
   absence de « Modifications » sans `request_id`, `trail` invalide, date en français, « — » d'une insertion, tri des
   scopes du jeton, échec et reprise des modifications ; l'annuaire testé opérateur par opérateur (un mutant du statut
   survivait à une recherche de sous-chaîne). Onze mutants survivants, tous tombés.
6. **Code en trop** : les scopes exclus du jeton personnel sont générés dans `scopes.gen.ts` depuis
   `dashboard.PersonalExcluded`, plus recopiés en TS ; `OperatorCell` partagé par l'écran Opérateurs et l'audit ;
   l'annuaire construit `OperatorRef` sans passer par le DTO complet.
7. **Gardés** :
   - `useClosedOn` et `Failure` restent dans `audit.tsx`, et les deux journaux ne fusionnent pas : leur adoption par
     Opérateurs et Rôles, ou un composant `Journal`, sortent du périmètre pour un gain modéré.
   - `Combobox` et `Choices` restent des briques du kit (278) : les écrans n'importent pas Base UI.
   - La restauration du focus de `Modal` reste : son mutant tombe, `finalFocus` ne joue pas au démontage.
   - `malformed` sans `InvalidParamFormatError` (paramètre requis manquant) n'a pas de test : aucune opération du
     contrat BFF n'a de paramètre de requête requis.
