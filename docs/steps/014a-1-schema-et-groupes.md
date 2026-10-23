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
