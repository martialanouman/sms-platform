# Step 014a-3 — Comptes

Plan : `docs/plan/composants.md` §014 à 020, troisième des trois PR de 014a (coupe : arbitrage 282). Arbitrages antérieurs :
1 à 323 (`docs/steps/014a-1-schema-et-groupes.md` porte ceux de toute la 014a, `docs/steps/014a-2-clients.md` ceux des
clients). Spec : `docs/spec/01-exigences.md` §1.1, §1.2 ; `docs/spec/08-dashboard.md` §8.8 ; ADR-0006. Contrat :
`contracts/openapi-admin.yaml` (tag `accounts`). Issue : #173 (reportée). Règles : `.claude/rules/tests.md`, `go-code.md`,
`contracts.md`. Mode : PR.

## Contexte

- 014a-1 (#172) a livré la table `accounts`, ses règles en base, le PATCH à trois états et la traduction des SQLSTATE.
- 014a-2 (#174) a livré les clients, leur cascade vers les comptes, la fiche client et l'aperçu de suspension.
- Aucune opération n'écrit encore un compte.

**Terminé quand**, contre un vrai Postgres puis dans le navigateur : `Propriétaire` crée un compte depuis la fiche
client, le renomme, règle ses canaux, ses sessions et ses opérations SMPP, le voit « Actif (client suspendu) » sous un
client suspendu, le suspend, le réactive, le clôt, en supprime un autre ; un nom pris est refusé (409) ; un client clos
refuse un compte neuf (422) ; chaque mutation est auditée sous son acteur.

## Arbitrages

Fable, 2026-10-23, sauf mention. Les décisions humaines du même jour sont marquées.

324. **Liste `/accounts`** : colonnes Nom (lien `/accounts/$id`), Client (`customer_name`, lien `/customers/$id`),
     Statut (331), Créé le ; aucun tri (curseur `(created_at, id)` décroissant). Filtres `Select` « Groupe : Tous »
     (`useGroups`, 311) et « Statut : Tous ». Le filtre client vit dans l'URL seule (`customer_id`), on y entre par
     « Tous les comptes » de la fiche client (338) ; posé, la barre montre « Client : {nom} » et « Retirer » (nom par
     `get-customer`, l'identifiant sans `customers:read`). URL : `customer_id`, `group_id`, `status`, `cursor`, `trail`.
325. **`customer_name` et `customer_status` dans `Account`** (humain, contre l'annuaire proposé) : la liste et la lecture
     joignent `customers` ; aucun annuaire de clients, aucun plafond. La fiche compte garde un `get-customer` pour sa
     remontée.
326. **Création depuis la fiche client seule** (304, spec 01 §1.2) ; `/accounts` n'a pas de « Nouveau compte ».
     Formulaire : le nom seul, les autres champs prennent leur défaut et se règlent sur la fiche. `AccountForm` sert
     aussi le renommage.
327. **Compte sous client clos refusé, à l'écran et en base** (humain, sur recommandation) : migration `00005`,
     trigger `BEFORE INSERT ON accounts` qui lève `check_violation` sous la contrainte `customer_is_closed` ; 422 sur
     `customer_id`. Sous client suspendu : accepté, effectif suspendu (ADR-0006, 295). « Nouveau compte » figé sur un
     client clos.
328. **Fiche compte `/accounts/$id`** : lien « ← {client} » vers `/customers/$id` au-dessus du titre (nom du compte) ;
     sans `customers:read`, « ← Client ».
329. **Quatre blocs, un « Modifier » par bloc**, chacun sur son opération : Identité → `update-account {name}` ;
     Canaux → `set-account-channels` ; Sessions → `set-account-session-limits`, types dans l'ordre fixe
     `[transmitter, receiver, transceiver]` ; Opérations SMPP → `set-account-smpp-ops`, seuls les drapeaux changés, rien
     de changé ferme sans appel ; sa modale dit « Les drapeaux se lisent au bind : les binds ouverts du compte sont
     déconnectés. » (la confirmation de 307).
330. **Dernier canal et dernier type de bind figés** et expliqués (« Au moins un canal reste activé. », « Au moins un
     type de bind. ») ; le 422 de la passerelle reste le filet.
331. **Statut effectif = le plus restrictif du client et du compte** (ADR-0006) : « Actif (client suspendu) ». Dans la
     colonne de `/accounts` (humain : `customer_status` au contrat), dans la section Comptes et sur la fiche.
332. **Actions de statut, toujours rendues** (315) : Suspendre (`suspend-account`, sans aperçu), Réactiver
     (`update-account {status: active}` ; sous client suspendu, « Son client est suspendu : le compte reste suspendu en
     pratique. »), Clôturer (danger, définitif), Supprimer (danger, retour à la fiche client ; aucun 409 en 014a, 296).
     Figés et expliqués comme pour un client ; Supprimer ouvert à un clos.
333. **Messages français fixes** (314) : suspension 409, réactivation ou clôture 422 → « Ce compte est clos
     définitivement. » ; création et renommage 409 → « Un compte de ce client porte déjà ce nom. » ; création 422 →
     « Ce client est clos. ».
334. **Traduction** par `fields` : `accounts_customer_id_name_key → name`, `accounts_customer_id_fkey → customer_id`,
     `customer_is_closed → customer_id`. `accounts_channel_check` sans champ (deux colonnes). Les CHECK de types de bind
     et de `max_sessions` sont inatteignables (huma borne avant).
335. **Compte d'un client clos** : tous clos (293), aucun ne naît (327) ; `closed_is_final` suffit.
336. **Dette #173 sans marque de plus** (317) : l'issue nomme déjà `suspend-account`, `update-account` et
     `set-account-smpp-ops`. Le 503 déclaré n'est pas émis (285).
337. **Contrat, premier commit** : `Account` gagne `customer_name` et `customer_status`, requis ;
     `AccountCreate.allowed_bind_types` gagne `default: [transceiver]` (298) ; `update-account` gagne `'409'` (renommage
     sur un nom pris). Défauts de création portés par huma, la base garde les siens.
338. **Section « Comptes » de la fiche client**, sous Identité : Nom, Statut (331), Créé le ; première page seule et
     « Tous les comptes » vers `/accounts?customer_id=` ; « Nouveau compte » en tête. Sans `accounts:read`, une phrase.
339. **Un `AccountPatch` pour `update-account`, les trois `set-*` et la suspension**, une requête `UpdateAccount` à
     `coalesce` ; pas de cascade, pas de port de sessions (285).
340. **Rail** : « Comptes » (`accounts:read`) entre « Clients » et « Groupes ». Libellés de statut : `customerStatus`.

## Design arrêté

- **Contrat** (premier commit) : 337 ; `make generate`.
- **Migration `00005`** : trigger `customer_is_closed` ; `Down` le retire.
- **`internal/adminapi`** : `Account` (avec `CustomerName`, `CustomerStatus`), `AccountFilter`, `AccountPatch` ; port
  `Accounts` dans `Tx` ; `AccountsPage`, `SuspendAccount`.
- **`internal/adminapi/postgres`** : `queries/accounts.sql` — liste et lecture jointes (filtre de groupe par
  `c.group_id`), écritures en `RETURNING id` puis relecture ; `fields` + 3 entrées ; `rate_limit` `null` (284).
- **`internal/adminapi/huma`** : les neuf opérations ; défauts par tags ; `smpp-ops` en `value[bool]`, `{}` → 422.
- **SPA** : `web/src/accounts.tsx` (`accountsKey`, `effective`, `AccountForm`, types de bind ordonnés) ;
  `_shell.accounts.index.tsx`, `_shell.accounts.$id.tsx` ; section Comptes de `_shell.customers.$id.tsx` ; rail.
- **Tests** : Postgres réel sous `admin_api_svc`, chaque opération, filtres, défauts, refus (409, 422), statut effectif
  servi, audit ; Vitest des écrans avec `axe`. Mutation manuelle, consignée ; preuve sur contabo169.
