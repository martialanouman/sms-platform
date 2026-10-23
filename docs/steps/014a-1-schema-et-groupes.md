# Step 014a-1 — Schéma et groupes de clients

Plan : `docs/plan/composants.md` §014 à 020, première des trois PR de 014a (coupe : arbitrage 282). Arbitrages
antérieurs : 1 à 281. Spec : `docs/spec/01-exigences.md` §1.1, §1.2 ; `docs/spec/07-plan-de-controle.md` §7.1 à §7.5 ;
`docs/spec/08-dashboard.md` §8.8 ; `docs/spec/09-contrats.md` §9.3 ; ADR-0006, ADR-0025 ; design 05 §4.3, design 07 D3.
Contrat : `contracts/openapi-admin.yaml` (tags `customer-groups`, `customers`, `accounts`). Origine : `go-gateway`
(`migrations/0001`, `0025`, `0027`, `internal/adminapi/customer*.go`, `accounts.go`), `go-gateway-bo` (écrans clients,
comptes, groupes). Issue : #61. Règles : `.claude/rules/tests.md`, `go-code.md`, `contracts.md`. Mode : PR.

Ce document porte la coupe de **toute la 014a** ; 014a-2 et 014a-3 y renvoient.

## Contexte

- `control_plane` n'a que `audit_log` et `audit_changes` ; `audit_capture` (I13, `pg_notify`) attend ses premières
  tables. Aucun service ne lit encore la configuration : `rest-api-svc` en 021, `session-manager-svc` en 025a,
  `smpp-server-svc` en 025b.
- `admin-api-svc` sert deux lectures (013a) ; aucune opération à corps, aucun PATCH. Le modèle d'erreur s'arrête à
  422 et 500.
- #61 (`go-live`, héritée de l'origine) : PATCH à trois états, imposé par le contrat.

**Terminé quand** (014a-1), contre un vrai Postgres puis dans le navigateur : `Propriétaire` crée, renomme, archive et
supprime un groupe depuis l'écran Groupes ; chaque mutation a sa ligne d'audit et son diff sous son acteur ; `null`
efface la description ; un nom pris rend 409 ; un lecteur de l'instantané voit la ligne ; `member_count` compte les
clients du groupe.

## Arbitrages

Fable, 2026-10-23, sauf mention. Les décisions humaines du même jour sont marquées.

282. **Coupe de 014a en trois PR verticales** (humain, sur recommandation), le schéma complet dans la première ; l'ordre
     suit les clés étrangères :
     - **014a-1, schéma et groupes** : migration des trois tables, classement d'audit, GRANT, plomberie partagée (trois
       états, traduction des SQLSTATE, `Principal` dans le contexte, 400 → 422), les six opérations `customer-groups`,
       écran Groupes, preuve d'instantané ;
     - **014a-2, clients** : `list`, `create`, `get`, `update`, `delete-customer`, `suspend-customer`,
       `get-customer-suspension-impact` ; écran Clients (filtre par groupe, fiche, suspension avec aperçu) ;
     - **014a-3, comptes** : `list`, `create`, `get`, `update`, `delete-account`, `suspend-account`,
       `set-account-channels`, `set-account-session-limits`, `set-account-smpp-ops` ; écran Comptes et fiche,
       navigation à deux niveaux.
283. **Opérations laissées au fichier** : `get`/`update-customer-content-policy`, `rotate-content-key` → 032 ;
     facturation du client → 031b ; `set`/`delete-account-rate-limit` et sender IDs → 015 ; identifiants et webhooks →
     014b ; `list-sessions`, `disconnect-session` → 025a, 026. La garde « servi ⊂ fichier » (230) les tolère.
284. **`Account.rate_limit`** rendu `null` jusqu'en 015, qui y joint `rate_limits` ; planifié, pas un report.
285. **Aucun port de déconnexion en 014a** : les mutations changent la base, rien d'autre ; `binds_to_disconnect` rend
     `[]`, vrai tant qu'aucun serveur SMPP n'existe (025b). Le 503 déclaré n'est jamais émis.
286. **Dette** (`debt`, porteur `session-manager-svc`, déclencheur 025a) : suspension, sortie d'`active` et changement
     de `query_sm`/`cancel_sm` ne déconnectent pas les binds ; l'aperçu ne les liste pas. Ouverte en 014a-2, marquée sur
     le constructeur de `SuspensionImpact`.
287. **`customer_groups`** : `id` (UUIDv7), `name` unique, `description` nullable, `status` (`active`, `archived`),
     `created_by uuid` nullable sans FK (le BFF a sa base ; origine 0027), `created_at`, `updated_at`. `member_count`
     calculé, jamais stocké.
288. **`customers`** : `id`, `name` (non unique : `create-customer` ne déclare pas 409), `status` (`active`,
     `suspended`, `closed`), `group_id` → `customer_groups` `ON DELETE SET NULL` (« detached, not deleted »),
     `max_age_otp`, `max_age_transactional`, `max_age_marketing` nullables, bornés par CHECK aux maxima de la spec 03
     §3.6, `created_at`, `updated_at`. Trois colonnes plutôt qu'un jsonb : l'objet est requis, ses champs seuls sont
     nullables, et un CHECK tient le secours.
289. **`accounts`** : `id`, `customer_id` → `customers` `ON DELETE RESTRICT`, `name`, `status`, `smpp_enabled`,
     `rest_enabled`, `query_sm_enabled`, `cancel_sm_enabled` (vrais par défaut), `allowed_bind_types text[]` (défaut
     `{transceiver}`, au moins un, sous-ensemble des trois), `max_sessions` (défaut 1, ≥ 0), `created_at`, `updated_at` ;
     `CHECK (smpp_enabled OR rest_enabled)`, `UNIQUE (customer_id, name)`, index sur `customer_id`.
290. **Clos définitif en base** : trigger `closed_is_final()` sur `customers` et `accounts`, à SQLSTATE propre, traduit
     en 409 là où l'opération le déclare (`suspend-*`), en 422 sur `status` sinon (`update-*` ne déclarent pas 409).
     `updated_at` tenu par un trigger `touch_updated_at()`, que le secours (§7.6) traverse aussi.
291. **Audit et GRANT** : les trois tables en `audit: diff` (design 05 §4.3) ; `admin-api-svc` en `select, insert,
     update, delete` sur les trois.
292. **PATCH `status: suspended` d'un client** cascade comme `suspend-customer` (spec 01 §1.2 : « suspendre un client
     suspend ses comptes », quel que soit le chemin) : `UPDATE accounts … WHERE status <> 'closed'`.
293. **Clôture d'un client** (humain) : même cascade, ses comptes non clos passent `closed`.
294. **Réactivation d'un client** (humain) : ses comptes restent suspendus, réactivés un à un ; l'écran le dit.
295. **Compte sous client suspendu** : sa réactivation est acceptée, l'effectif reste suspendu (ADR-0006), l'écran
     l'affiche. Suspendre un suspendu : 200. Un groupe `archived` ne porte aucune règle (spec 01 §1.1) ; le sélecteur
     n'offre que les actifs, plus celui déjà posé.
296. **Suppression d'un client** (humain, révise la proposition de Fable) : seulement **sans compte et sans trafic** ;
     sinon 409, et l'exploitant suspend ou clôt. En 014a, le compte seul (`ON DELETE RESTRICT`). Le trafic suit en
     015 sans code de plus : `sender_ids.customer_id` en `ON DELETE RESTRICT`, et un sender ID qui a servi ne se
     supprime pas (contrat, `delete-sender-id`) ; tout MT passe par un sender ID actif du client (spec 01 §1.1). Inscrit
     au plan §015. `delete-account` n'émet aucun 409 en 014a ; 014b décide pour ses enfants.
297. **Traduction des SQLSTATE**, une fonction dans `internal/adminapi/postgres` : FK violée à l'écriture → 422 sur le
     champ ; FK violée à la suppression → 409 ; `unique_violation` → 409 ; `check_violation` → 422 sur le champ, par nom
     de contrainte ; clos définitif → 290.
298. **`allowed_bind_types`** absent à la création → `[transceiver]` ; le contrat gagne `default: [transceiver]`
     (additif), en 014a-3, avant le code.
299. **`max_age_override`** : « la plateforme » est le tableau de la spec 03 §3.6 (constantes) ; les maxima du contrat
     les égalent ; les CHECK de 288 suffisent.
300. **400 → 422 `validation_error`** par `huma.NewError` (spec 09 : une requête mal formée est un 422 ; 276 l'a fait au
     BFF). **413 `payload_too_large`** reste, non déclaré, comme le 503 des middlewares (230) ; `MaxBodyBytes` à
     64 KiB. Le contrat ne change pas.
301. **Les règles vivent en base** (clos définitif, un canal, bornes, cardinalité, unicité) ; le Go ne porte que la
     cascade et la traduction. Pas de contexte `internal/customers` : le statut effectif et l'admission au bind
     arrivent avec 021 et 025b. `internal/adminapi` gagne les entités et étend `Tx` ; `postgres` les implémente
     (sqlc) ; `huma` les sert.
302. **Trois états** : un type générique (absent, `null`, valeur) décodé du JSON, son schéma nullable ; côté SQL, un
     drapeau par champ : `col = CASE WHEN @set_col THEN @col ELSE col END`. `authorize` pose le `Principal` dans le
     contexte ; `created_by` = `sub` s'il est un UUID.
303. **Preuve d'instantané** dans le test Postgres : la mutation passe par le handler réel sous `admin_api_svc`, puis
     `snapshot.Open` sous un rôle lecteur créé par le test voit la ligne ; `audit_changes` porte l'acteur. Les GRANT
     des lecteurs futurs viennent avec leurs services (design 07 D3).
304. **Rail** : groupe « Clients » au-dessus d'« Administration » : Clients (`customers:read`), Comptes
     (`accounts:read`), Groupes (`customer-groups:read`), chaque entrée avec son écran ; sans la permission,
     `Forbidden` (281). Deux niveaux : la fiche client porte ses comptes ; la fiche compte remonte au client.
305. **Filtres** : `Combobox` du kit pour le groupe sur `/customers` et `/accounts`, statut en filtre, l'URL porte
     filtres et `trail` (279).
306. **Suspension** : `Confirmation` qui charge l'aperçu (« N comptes seront suspendus, M clos restent clos ») ; aperçu
     en échec : la confirmation reste possible, l'impact « indisponible ».
307. **Désactivé et expliqué** (spec 08 §8.8) : écriture sans `:write` (bulle) ; clos : formulaires figés, « clos
     définitivement » ; « Suspendre » figé si suspendu ; dernier canal figé ; changement de `query_sm`/`cancel_sm`
     confirmé. Le formulaire n'envoie que les champs modifiés ; vider un champ nullable envoie `null`.
308. **Rôles** : seul `Propriétaire` écrit par défaut (`internal/dashboard/roles.go`) ; la preuve se joue sous lui.
309. **Dettes** : #61 soldée ici (première implémentation du PATCH ; le motif de 302 vaut pour les tables suivantes).
     Ouverte en 014a-2 : 286. Sans effet : #119 (même report pour trois tables de plus), #162, #169 (aucun MSISDN).

Retour humain de la preuve navigateur, 2026-10-23 :

310. **Barre d'outils** : les filtres à gauche, l'action à droite, à la même hauteur ; le filtre n'a pas de libellé au-dessus,
     son déclencheur dit « Statut : Tous » (nom accessible « Statut », contenu dans le texte visible). `Toolbar` gagne
     `filters`.

## Design arrêté

- **Migration** `migrations/control_plane/00004_customers.sql` : `customer_groups`, `customers`, `accounts` (287 à
  289), triggers `closed_is_final`, `touch_updated_at` et `audit_capture` sur les trois ; `Down` les retire.
  `audit.yaml` : les trois en `diff`. `contracts/grants/admin-api-svc.yaml` ; `migrations/grants/control_plane.sql`
  régénéré.
- **`internal/adminapi`** : `CustomerGroup`, filtre de liste ; `Tx` gagne les méthodes des groupes ; type à trois états
  (302) ; `Principal` dans le contexte.
- **`internal/adminapi/postgres`** : requêtes sqlc des groupes (liste par curseur avec `member_count`, statut filtré ;
  lecture ; création ; mise à jour à trois états ; suppression ; clients d'un groupe, schéma `Customer` lu sur la table
  déjà migrée) ; traduction des SQLSTATE (297).
- **`internal/adminapi/huma`** : les six opérations `customer-groups` ; 400 → 422 ; `MaxBodyBytes`.
- **`docs/plan/composants.md`** §015 : la règle de suppression du client (296).
- **SPA** : `web/src/routes/_shell.groups.tsx` (liste, filtre de statut, création, modification, archivage,
  suppression confirmée) ; rail (304).
- **Tests** : Postgres réel sous `admin_api_svc` — chaque opération, diff et acteur, `null` efface, absent garde, nom pris
  409, 404, curseur, `member_count`, clients d'un groupe, suppression qui détache, instantané (303), corps illisible
  422 ; garde de contrat et complément de scope étendus d'office ; contraintes de la migration (canal, bornes, clos
  définitif, cardinalité, `RESTRICT`) ; Vitest de l'écran avec `axe`. Mutation manuelle, consignée.

## Écarts au design, constatés à l'implémentation

- **`created_by` posé par la base** : défaut `control_plane.actor_operator()`, l'acteur s'il est un UUID, NULL sous
  `breakglass:` ou `service:` ; le secours (§7.6) le tient sans code, et `authorize` ne pose pas de `Principal` dans le
  contexte (302 révisé).
- **FK en `RESTRICT`** : la suppression refusée lève 23001 (`restrict_violation`), pas 23503 ; la traduction de 014a-2 lit
  ce code.
- **Traduction des SQLSTATE réduite** à ce que les groupes atteignent : 23505 → 409 sur le champ, ligne absente → 404.
  Les autres cas de 297 viennent avec les opérations qui les atteignent (014a-2, 014a-3).
- **`null` sur un champ non nullable** (`name`, `status`) : huma ignore tout `null` d'un champ facultatif à la validation ;
  un type `value[T]` le refuse au décodage, 422 (correctif de revue).
- **`MaxBodyBytes`** : le défaut d'huma (1 Mio) reste ; 64 Kio n'apportait rien de prouvable. Le 413 sort
  `payload_too_large`, et le 400 de huma sort 422.
- **Pagination partagée** : `page` du cœur sert l'audit, les groupes et les clients d'un groupe ; ordre `(created_at,
  id)` décroissant.
- **SPA** : `Select` du kit sur Base UI (filtre de statut, 278) ; `Pager`, `Failure`, `useClosedOn` sortis
  d'`audit.tsx` vers `list.tsx`. Un 409 de création ou de renommage dit « Un groupe porte déjà ce nom. ».

## Mutations

À la main, par un script de copie, mutation, test et restauration (`cmp`). Migration 15/15, puis `created_by` 2/3 ;
couche huma (erreurs, trois états, câblage) 6/7 ; adaptateur 3/3 ; cœur 1/1 ; SQL généré 11/11 après un test renforcé
(`member_count` d'une lecture) ; écran 19/20 après trois tests ajoutés (nom seul, refus gardé dans la modale, « Tous ») ;
`Select` 2/2 ; rail 1/1. Équivalents gardés : l'ancre `$` de `actor_operator` (aucun acteur n'est un UUID suivi
d'autre chose : le `sub` est vérifié, 231) ; `'' → undefined` du filtre de statut, que `validateSearch` normalise déjà.
Retiré parce qu'équivalent : `Nullable = true` du schéma du champ à trois états (huma ignore le `null` d'un champ
facultatif). Un survivant venait d'un filtre `-run` trop étroit : rejoué, tué.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant.

1. **`null` sur `name` ou `status` rendait 200** (moyenne) : refusé au décodage par `value[T]`, 422 ; `minLength` et
   `enum` restent appliqués à travers le type (mutations tuées).
2. **Tests creux** : pagination à égalité de `created_at` (lignes d'un même `INSERT`), acteur et ligne d'audit de l'`update`
   et du `delete` (le contenu du diff est prouvé par les tests de capture de 006a), `message` du refus, planchers des trois âges ; à l'écran, chemin de la suppression,
   description envoyée à la création, liste rafraîchie après archivage et suppression, session close sur un 401, refus de
   la passerelle affiché, statut inconnu de l'adresse écarté. Les huit mutants de la revue sont tués.
3. **Code en trop retiré** : `widen` (override sqlc en `*int`), `toGroup` et le recopiage de `maxAge` (conversions
   directes), `optional`, le `PageHeader` rendu deux fois.
4. **Gardés, avec raison** :
   - le schéma servi du champ à trois états n'est pas nullable : sans effet sur une requête (huma admet le `null` d'un
     champ facultatif), et la garde de contrat ne compare pas les schémas ;
   - un octet NUL dans un nom rend 500 générique, sans fuite ; l'écran ne le produit pas ;
   - `MaxBodyBytes` reste au défaut d'huma (écart ci-dessus) ;
   - `Cursor` et `Limit` restent écrits dans chaque entrée : un type embarqué pour huit lignes ;
   - le port `Groups` reste nommé : 014a-2 et 014a-3 y ajoutent `Customers` et `Accounts`.

## Correctifs de revue, tour 2

Une revue des correctifs du tour 1. Aucun défaut fonctionnel.

1. **Prose** (basse) : les tests ajoutés prouvent l'acteur et la ligne d'une mise à jour et d'une suppression, pas le
   contenu du diff ; corrigé ci-dessus.
2. **Gardé** : un `null` refusé sort avec `field: "body"` et non `body.name` ; l'erreur vient du décodage, pas du
   validateur. Statut et code justes, et l'écran n'envoie jamais ce `null`.

## Preuve sur `contabo169`

Jouée le 2026-10-23 depuis la branche, sur le cluster en v0.29.0 : images `migrate`, `catalog-apply`, `admin-api-svc`
et `dashboard` construites localement (même `Dockerfile` que GoReleaser), importées sous `proof-014a-1`, puis
`dashboard` sous `proof-014a-1-2` (retour 310) et `proof-014a-1-3` (correctif ci-dessous) ; timer suspendu. `migrate`
a appliqué la migration 4, `catalog-apply` les GRANT d'`admin_api_svc` sur les trois tables. Navigateur humain, puis
Chromium (`agent-browser`) par tunnel SSH vers `port-forward`, sur `localhost:8080`, compte de test `ada.preuve` passé
`Propriétaire` par l'humain, mot de passe et TOTP posés par un lien de réinitialisation :

1. Rail « Clients › Groupes » ; filtre « Statut : Tous » à gauche, « Nouveau groupe » à droite, même hauteur
   ([capture](014a-1/liste.png)).
2. Création d'« Assurances » avec sa description ([capture](014a-1/creation.png)) ; un nom pris dit « Un groupe porte
   déjà ce nom. » dans la modale ([capture](014a-1/nom-pris.png)).
3. Description vidée : la colonne dit « — » (`null` envoyé). Archivage confirmé ; « Statut : Archivés » porte
   `?status=archived` et ne liste que le groupe archivé ([capture](014a-1/archives.png)) ; « Réactiver ».
4. Suppression : « Ses clients (0) sont détachés du groupe, pas supprimés. » ([capture](014a-1/suppression.png)).
   **Défaut trouvé** : le `DELETE` partait sans `Content-Type`, refusé en 415 par le garde CSRF du BFF (« La requête
   doit être en application/json. ») ; les tests ne le voyaient pas, le double de `fetch` ignorant ce garde. Corrigé :
   le client `gw` pose `application/json` sur toute écriture, et `stubFetch` rend 415 comme le BFF ; rejouée, la
   suppression passe.
5. Audit : chaque mutation a sa ligne sous Ada (`create`, `update` ×5, `delete`), le `DELETE` refusé par le BFF n'en a
   pas ; « Modifications » de la suppression montre la ligne de `customer_groups`, `created_by` égal à l'acteur
   ([capture](014a-1/audit-diff.png)).
6. axe (WCAG 2.1 AA) : aucune violation sur la liste ni sur la modale ; un `aria-hidden-focus` « à revoir » sur les
   gardes de focus de Base UI, commun à toutes les modales du kit.

L'écran refusé sans `customer-groups:read` est prouvé par Vitest, pas rejoué dans le navigateur. Le cluster reste sur
`proof-014a-1-3`, timer suspendu ; il se réarme au merge.
