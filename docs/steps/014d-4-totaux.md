# Step 014d-4 — Totaux des listes paginées

Dernière des quatre PR de 014d (coupe : arbitrage 426). Les arbitrages de toute la 014d sont dans
`docs/steps/014d-1-operateurs.md` (426 à 451) ; cette PR applique 427 à 430 (part `total`), H6, 448 et 449 pour sa
part. Elle révise 400 (014c) : le total de pagination entre au contrat. Mode : PR.

**Terminé quand**, dans le navigateur sur contabo169 : les pieds des listes Clients, Comptes et Groupes disent « N entrées affichées sur T », T étant le nombre de lignes qui répondent aux filtres,
le même sur toutes les pages ; le contrat Admin servi déclare `total` requis sur les trois pages.

## Design arrêté

- **Contrat Admin d'abord** (`contracts/openapi-admin.yaml`) : `total: { type: integer, minimum: 0 }` requis sur
  `CustomerPage`, `CustomerGroupPage`, `AccountPage` (`list-group-customers` l'hérite par `CustomerPage`) ; description
  « nombre de lignes répondant aux filtres, indépendant du curseur » ; ni sur les pages de l'audit (H5).
  `web/src/admin.gen.ts` régénéré (`go generate ./...`).
- **Requêtes sqlc** (`internal/adminapi/postgres/queries/`) : `CountGroups`, `CountCustomers`, `CountGroupCustomers`,
  `CountAccounts`, mêmes filtres que leurs listes, sans curseur ni limite ; pas de `count(*) OVER ()` (428).
- **Cœur** (`internal/adminapi`) : les pages de groupes, de clients, de clients d'un groupe et de comptes rendent leur
  total, lu par le port dans la même transaction que la page (`counted()` qui appelle `page()` puis le compte ;
  `page()` ne bouge pas).
- **huma** (`internal/adminapi/huma`) : `countedPageOutput[T]` `{data, next_cursor, total}` à côté de `pageOutput[T]`,
  qui reste à l'audit ; les quatre listes passent à la nouvelle enveloppe, `total` requis dans le schéma servi.
- **Tests Go** (429) : par liste, `limit=1` → une ligne, curseur posé, `total` entier ; même `total` en page 2 ; un
  filtre (statut, groupe, client) → le compte filtré ; un test lit le contrat servi et exige `total` dans `required`
  des trois schémas.
- **`web/src/list.tsx`** : `Pager({ …, total? })` ; « N entrées affichées sur T » quand `total` est fourni (« 1 entrée
  affichée sur T »). Clients, Comptes et Groupes passent `total` (aucun écran ne lit encore `list-group-customers`).
- **Tests SPA** : pied « sur T » sur chaque liste ; mutation consignée ; preuve sur contabo169, captures dans
  `docs/steps/014d-4/`.

## Écarts au design, constatés à l'implémentation

- **Trois requêtes de compte, pas quatre** : `list-group-customers` passe par `CustomersPage` avec le filtre de groupe ;
  `CountCustomers` le compte, `CountGroupCustomers` serait un doublon.
- **Pas de `counted()` dans le cœur** : chaque page appelle `page()` puis le compte de son port, trois lignes ; un
  générique coûterait plus qu'il n'épargne. `counted` est l'enveloppe huma.
- **Les tests de pagination existants exigent le total** à chaque page (même total d'une page à l'autre, égal au
  nombre de lignes parcourues, pour chaque filtre) au lieu d'un test neuf par liste.
- **Correctif de revue 1** : le test du pied des Groupes passe un total distinct du nombre de lignes (« 2 entrées
  affichées sur 17 ») ; un total pris dans la page le fait tomber.
- **Instantané du compte, accepté** : la page et son compte sont deux instructions d'une même transaction en READ
  COMMITTED ; une écriture entre les deux peut décaler le total d'une ligne, le temps d'une requête. Passer la
  transaction de lecture en REPEATABLE READ toucherait toutes les transactions d'admin-api-svc pour un écart passager
  et sans effet sur les données : non retenu.

## Preuve sur contabo169

2026-10-24, admin-api-svc et dashboard construits depuis la branche, déploiement automatique suspendu le temps de la
preuve (un déploiement déjà en cours a d'abord remis admin-api-svc sur la version publiée ; redéployé). Compte de
preuve `ada.preuve`, mot de passe et second facteur neufs, identifiants effacés ensuite.

- API servie : `GET /admin/customers?limit=1` → une ligne, `next_cursor` posé, `total: 3` ; la page suivante → une
  ligne, `total: 3`.
- Pieds : Clients « 3 entrées affichées sur 3 » (`customers.png`), Comptes « 6 entrées affichées sur 6 »
  (`accounts.png`), Groupes « 1 entrée affichée sur 1 » (`groups.png`) ; filtre `?status=active` sur Clients : « 1
  entrée affichée sur 1 » (`customers-actifs.png`) ; axe sans violation.
