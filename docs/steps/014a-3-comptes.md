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

## Écarts au design, constatés à l'implémentation

- **La fiche compte ne lit pas le client** (révise 328) : `Account` porte `customer_name` et `customer_status` (325), qui
  suffisent au lien de remontée « ← {client} » et au statut effectif ; aucun `get-customer`, aucun cas sans
  `customers:read`.
- **`--default-non-nullable=false`** pour le contrat Admin (`doc.go`) : `openapi-typescript` 7 rendait obligatoire tout
  champ à défaut, y compris dans un corps de requête ; la création n'aurait pu envoyer le nom seul (326).
- **Clé `accountsKey` sous `customersKey`** : une mutation d'un client (renommage, suspension et clôture en cascade)
  recharge les comptes affichés sans câblage ; une mutation d'un compte ne recharge que les comptes.
- **Création : la conversion en `Account` complet est faite par huma** (défauts posés par les tags) ; le port prend un
  `Account` plutôt qu'un patch.
- **`closedConflict`** partagé par `SuspendCustomer` et `SuspendAccount`, sans condition de champ : une suspension ne peut
  être refusée que sur le statut.
- **`FOR SHARE` dans le trigger `customer_is_closed`** : une création concurrente d'une clôture du client attend sa fin
  et voit le client clos ; sans le verrou, le compte naissait actif sous un client clos, après la cascade.
- **Le 422 des canaux n'est pas traduit** (révise 330) : la phrase « Au moins un canal reste activé. » est affichée en
  permanence sous les cases ; la répéter en refus doublait le texte. Le 422 garde le message de la passerelle.
- **`Choice.disabled`** au kit, `Toolbar.children` facultatif, `.ui-filter` pour le jeton « Client : … ».
- **Un PATCH `{}` réécrit la ligne** (`updated_at`), audité comme toute mise à jour, comme pour un client.

## Mutations

À la main, par le script de copie, mutation et restauration (`cmp`). SQL généré 14/14 ; traduction 3/3 ; adaptateur
2/2 ; cœur 2/2 (le survivant de `closedConflict` a fait retirer la condition de champ) ; migration 2/2 (le survivant
`FOR SHARE` a fait écrire le test de course) ; huma 8/8 (défauts compris). SPA : module partagé 6/6 après deux tests de
rechargement et un cas « suspendu sous suspendu » ; liste 6/7, le survivant équivalent de 014a-2 (`validateSearch`) ;
fiche 13/13 après trois tests et le retrait du 422 doublé ; section de la fiche client 6/6 après deux tests ;
`Choice.disabled` 1/1.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant.

1. **`allowed_bind_types: null` accepté** (moyenne) : huma rend un tableau nullable et ignore `null` sur un champ
   facultatif ; `session-limits` répondait 200 sans rien changer, la création posait le défaut. `nullable:"false"` sur
   `session-limits`, `value[[]string]` à la création (le défaut `[transceiver]` posé par le handler, le tag `default` ne
   s'applique pas à ce type).
2. **`max_sessions` au-delà de l'`integer`** (moyenne) : 500 au lieu de 422. Le contrat gagne `maximum: 2147483647` sur
   les trois schémas, huma l'applique aux entrées, le champ de la SPA porte `max`.
3. **Le 422 de création** (basse) : traduit à la création seule, « Ce client est clos ou n'existe plus. » (révise 333 :
   la FK et le trigger tombent tous deux sur `customer_id`) ; un autre 422 au renommage garde le message de la
   passerelle.
4. **Groupes de cases sans nom** (basse) : `Choices` gagne `label` et `describedBy`, reliés au refus.
5. **Tests ajoutés** : statut et statut du client servis par la liste ; écritures partielles qui épargnent les autres
   champs ; drapeaux croisés à la création et à l'affichage ; canaux envoyés sans croisement ; dernière opération SMPP
   désactivable ; corps du renommage ; 422 de création, de réactivation et de clôture ; filtre client gardé et curseur
   retiré au changement de filtre ; « Modifier » figés sans `accounts:write` ; types de bind `[]`, inconnus, `null`.
6. **Code en trop retiré** : `SessionsForm` refaisait `Settings` (désormais un champ enfant de `Settings`) ; le rang des
   statuts et « (client clos) », inatteignables (335) ; les casts des `coalesce` ; une constante à un seul usage ; la
   borne du DTO de réponse, qu'aucune garde ne lit.
7. **Écart 328 confirmé** par les deux revues : le code suit 325 (humain), la fiche affiche `customer_name`.

## Correctifs de revue, tour 2

Une revue des correctifs du tour 1. Aucun bloquant.

1. **Schéma servi de la création nullable sur `allowed_bind_types`** (basse) : `value[[]string]` délègue au schéma d'un
   `[]string`, nullable chez huma ; `nullable:"false"` l'aligne sur le contrat (le refus de `null` tenait déjà par
   `value`). Le `default: [transceiver]` du contrat n'est pas servi : le tag fait paniquer huma sur ce type, le handler
   pose le défaut.
2. **Description du groupe par le refus** : testée.
