# Step 012a — Rôles et autorisation

Plan : `docs/plan/composants.md` §012, première des cinq PR de 012. Arbitrages antérieurs : 1 à 137
(`docs/steps/011*.md`). Spec : `docs/spec/08-dashboard.md` §8.3, §8.5, §8.9 ; `docs/spec/07-*.md` §7.4 ;
`docs/design/05-*.md` §3.3. Contrat : `contracts/openapi-bff.yaml` (`me`, `listAccessLog`, `Role`, `updateRole`).
Origine : `martialanouman/go-gateway-bo`, `internal/permissions/roles.go`, `orphans_test.go`. Règles :
`.claude/rules/tests.md`. Mode : PR.

Ce document porte la coupe de **toute la 012** ; 012b à 012e y renvoient.

## Contexte

- Aucune table de rôles. `internal/dashboard/http/http.go` tient `var held []string` (nil) : toute opération à
  permission rend 403, `listAccessLog` comprise. Aucune garde `mfa_required` : une session non élevée atteindrait
  toute opération si elle en avait la permission.
- `internal/platform/scopes` liste déjà, générés des contrats, les scopes de l'Admin API (`All`) et les permissions
  du BFF (`Permissions`).
- `Bootstrap` crée l'opérateur initial sans rôle ; l'environnement de test a déjà le sien.
- Le `CHECK` de `access_log.event` porte déjà `roles_changed`.

**Terminé quand** : contre un vrai Postgres, l'opérateur du bootstrap détient `Propriétaire` et `GET /auth/me` porte
l'union des permissions de ses rôles ; une session non élevée d'un opérateur `Audit` reçoit 403 `mfa_required` sur
`GET /access-log` et 200 sur `GET /auth/me`, puis 200 sur `GET /access-log` une fois élevée ; une session `Reporting`
n'est confinée par aucune garde ; le test des orphelins est vert. Sur l'environnement de test, l'opérateur existant
reçoit `Propriétaire` au déploiement, et la ligne `roles_changed` est au journal.

## Arbitrages

Fable, 2026-10-21. 012a est conduite sans arbitrage humain ; la coupe (138) est soumise à l'humain avec la PR.

138. **Coupe de 012 en cinq** :
     - **012a, rôles et autorisation** : tables, rôles par défaut, test des orphelins, union des permissions,
       garde `mfa_required`, `Propriétaire` au bootstrap ; aucun handler neuf.
     - **012b, opérations opérateurs et rôles** : `listOperators`, `createOperator`, `updateOperator`,
       `setOperatorRoles`, `requestOperatorAccessLink`, `listRoles`, `createRole`, `updateRole`, `deleteRole`,
       `self_lockout`, faits du §8.9 ; preuve `curl`.
     - **012c, jetons** : clé Transit, politique et rôle Kubernetes du BFF, émetteur (ADR-0019), JWKS sur l'écouteur
       interne mTLS (allowlist `admin-api`), NetworkPolicy, `issueGatewayToken` ; preuve depuis un pod `admin-api`.
     - **012d, coquille et écran Opérateurs** ; **012e, écrans Rôles et jeton personnel** ; preuve navigateur.
139. **`listSessions`, `deleteSession`** : hors de 012, issue #149 (`debt`) ; les permissions `sessions:*` existent et
     sont placées (orphelins).
140. **Rôles par défaut** : une table Go du cœur, semée à chaque démarrage par un upsert sur le nom qui réécrit
     description, permissions et `is_default` ; l'`id` survit, les détentions aussi. Le contrat dit « re-seeded on
     every deployment » ; une migration ne s'applique qu'une fois. `Propriétaire` est dérivé (`scopes.All` ∪
     `scopes.Permissions`), jamais listé.
141. **Stockage** : `roles (id uuidv7, name unique, description, permissions text[], is_default)`,
     `operator_roles (operator_id, role_id)`. L'union est calculée par `SessionOf` à chaque requête, jamais stockée en
     session : un changement de rôles vaut au prochain appel. La validation d'une permission écrite vient en 012b.
142. **Bootstrap** : si aucun opérateur **actif** ne détient `Propriétaire`, le rôle est posé à l'opérateur de
     `DASHBOARD_BOOTSTRAP_EMAIL`, neuf ou non, avec une ligne `roles_changed`. Sans condition, il ressusciterait à
     chaque déploiement un rôle qu'un propriétaire aurait retiré.
143. **Garde MFA** (§8.3) : une permission demande la MFA si elle finit par `:write`, si elle est l'un des dix scopes
     sensibles du §7.4 (`content:read`, `content:write`, `content:erase`, `gdpr:erase`, `msisdn:reveal`,
     `cdr:export_bulk`, `audit:read`, `routing-scripts:publish`, `suppressions:delete`, `billing:topup`) ou une
     permission du BFF. `stream:read` est ordinaire. Une session non élevée dont l'union en porte une reçoit 403
     `mfa_required` sur toute opération hors `/auth/*`. Le contrôle de permission suit la garde.
144. **`noc` sans délai d'inactivité** : reporté en 026, issue #148 (`debt`). `noc` est semé avec le délai ordinaire.
145. **Table des rôles par défaut** (§8.5 ; « comptes » = `accounts`, `customers`, `customer-groups`, `sender-ids` ;
     « trace » = `traffic:read` ; toute portée qui lit le trafic porte `stream:read`) :
     - `Exploitation` : lecture et écriture de `routes`, `exact-routes`, `connectors`, `smpp-sessions`, `antispam`,
       `routing-scripts`, `sender-rewrite`, `inbound-numbers`, `suppressions`, `opt-out-keywords`, `dead-letters`,
       `platform` ; `routing-scripts:publish`, `msisdn:reveal`, `cdr:export_bulk`, `sessions:read`,
       `sessions:write` ; lecture de `accounts`, `customers`, `customer-groups`, `sender-ids`, `credentials`,
       `billing`, `cdr`, `jobs`, `traffic` ; `stream:read`.
     - `Scripts` : `routing-scripts:read`, `routing-scripts:write`, `traffic:read`, `stream:read`.
     - `Support` : lecture de `accounts`, `customers`, `customer-groups`, `sender-ids`, `routes`, `exact-routes`,
       `connectors`, `smpp-sessions`, `cdr`, `billing`, `traffic` ; `msisdn:reveal`, `stream:read`.
     - `Finance` : `billing:read`, `billing:write`, `billing:topup`, et les lectures de `Support` sans
       `msisdn:reveal`.
     - `Reporting` : `billing:read`, `traffic:read`, `stream:read`.
     - `Clientèle` : lecture et écriture de `customers`, `accounts`, `credentials`, `customer-groups`, `sender-ids`
       (donnée du client, §1.2) ; `billing:read`, `billing:write`.
     - `Conformité` : `suppressions:read`, `suppressions:write`, `suppressions:delete`, `opt-out-keywords:read`,
       `opt-out-keywords:write`, `inbound-numbers:read`, `gdpr:erase`, `content:erase`, `msisdn:reveal`,
       `cdr:export_bulk`, lecture de `accounts`, `customers`, `customer-groups`, `sender-ids`, `cdr`.
     - `Audit` : `audit:read`.
     - `noc` : tout `*:read` sauf `audit:read` et `content:read` ; `stream:read`.
146. **Orphelins** (§8.5) : toute permission de `scopes.All` ∪ `scopes.Permissions` est détenue par un rôle par
     défaut autre que `Propriétaire`, sauf `content:read`, `content:write`, `operators:*`, `roles:*`,
     `gateway:personal_token`, qui ne le sont par aucun. Le test lit la table Go ; une table de vérité transcrite du
     §8.5 la tient rôle par rôle.
147. **Descriptions** : une phrase française par rôle, tirée de sa ligne du §8.5.
148. **#146** (cookie élevé capturé) reste ouverte : son déclencheur est la mise en production ; 012a n'ajoute aucune
     opération qui l'aggrave.

## Design arrêté

- **`migrations/dashboard/00004_roles.sql`** : `roles`, `operator_roles` (sans cascade : un rôle détenu ne se
  supprime pas, un opérateur jamais) ; `audit.yaml` (`exempt`, les faits vont au journal d'accès) ; `contracts/grants/dashboard.yaml`
  et la sortie régénérée.
- **Cœur `internal/dashboard`** : `roles.go` (rôles par défaut, `Owner`, `RequiresMFA`) ; `session.go`
  (`Current.Permissions`) ; `bootstrap.go` (semis, propriétaire) ; `store.go` (ports `SeedRole`,
  `EnsureOwner`).
- **`internal/dashboard/postgres`** : requêtes sqlc du semis et du propriétaire ; `SessionOf` rend l'union triée.
- **`internal/dashboard/http`** : `held` disparaît ; la garde `mfa_required` hors `/auth/*`, puis la permission ;
  `me` rend `permissions`.
- **Marques** : `debt: #148` sur `IdleTimeout` ; `debt: #149` sur `include-operation-ids`.
- **Tests** : table de vérité du §8.5 ; orphelins ; `RequiresMFA` ; Postgres réel : semis idempotent et réécrit,
  bootstrap neuf, bootstrap existant, propriétaire déjà présent (rien posé), propriétaire désactivé (posé), union de
  deux rôles ; HTTP : `Audit` confiné puis élevé, `Reporting` libre, permission absente 403 `forbidden_scope`.
  Mutation manuelle, consignée.
- **Preuve sur `contabo169`** : déploiement de la branche ; `roles_changed` au journal ; `me` élevé porte l'union de
  `Propriétaire` ; `GET /access-log` 403 non élevé, 200 élevé.

## Écarts au design, constatés à l'implémentation

- **Pas de `Current.Confined`** : la garde de `internal/dashboard/http` appelle `RequiresMFA` sur l'union ; seule elle sait
  si l'opération est sous `/auth/*`.
- **`content:write` n'est pas listé parmi les sensibles du cœur** : le suffixe `:write` le couvre déjà.

## Mutations

Trente-deux, à la main, sur la table des rôles, `RequiresMFA`, le SQL généré, l'adaptateur, le bootstrap et la garde ;
toutes tombent sauf une : la copie de l'union dans `me` (la base rend `{}`, jamais `NULL`), retirée.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant.

1. **Union non filtrée par opérateur** (haute, tests) : chaque test n'avait qu'un opérateur ; `WHERE true` survivait.
   Un second opérateur, `Propriétaire`, tient désormais le test d'union.
2. **Code en trop retiré** : la cascade de `operator_roles` (un rôle détenu ne se supprime pas, 012b) ; `content:write`
   des sensibles ; le test HTTP d'union, redondant avec le cœur et `me`.
3. **Tests** : `noc` attendu en liste littérale, plus recalculé comme le code ; `me` comparé au contenu, plus compté ;
   `me` sans rôle rend `[]`.
4. **Gardé, soumis à l'humain** : `sessions:read` et `sessions:write` à `Exploitation` (145, lecture de Fable : la ligne
   du §8.5 peut ne viser que les sessions SMPP ; en 012b, un opérateur `Exploitation` fermera la session d'un autre).
5. **Gardé** : le semis ne journalise rien (un déploiement n'est pas un fait d'opérateur, §8.9) ; `Propriétaire` peut
   aller à l'opérateur du bootstrap désactivé, ce qui ne rend aucun propriétaire actif (142, à la lettre) ; un compte
   actif jamais activé compte comme propriétaire ; un futur rôle par défaut homonyme d'un rôle taillé l'écraserait,
   à trancher quand 012b ouvre `createRole`.

## Preuve sur `contabo169`

Jouée le 2026-10-21 depuis la branche, sur le cluster en v0.17.0 : images `migrate`, `catalog-apply` et `dashboard`
construites par GoReleaser en snapshot, importées sous `proof-012a` ; timer suspendu, puis réarmé, et dashboard rendu à
la v0.17.0, compatible avec le schéma 4 (tables seulement ajoutées).

1. `migrate` a appliqué la migration 4, `catalog-apply` les GRANT de `roles` et `operator_roles`.
2. Au démarrage : dix rôles, tous par défaut ; l'opérateur existant, sans rôle depuis 011, détient `Propriétaire` ; une
   ligne `roles_changed` au journal.
3. Compte réinitialisé par un lien `reset` (Mailpit), puis par `curl` sur `port-forward` : session non élevée, `me`
   rend 57 permissions (les 50 scopes et les 7 permissions) et `GET /access-log` 403 `mfa_required` ; application
   enrôlée, session élevée par son code : `GET /access-log` 200.
4. Journal, du plus récent : `mfa`, `factor_enrolled`, `login`, `access_link_used`, `factor_removed`,
   `roles_changed`.

Non prouvé sur le cluster : une session `Reporting` libre, `Audit` confiné ; les tests HTTP contre Postgres réel les
couvrent (`TestASessionNotElevatedReachesOnlyAuthWhenItsRolesRequireMFA`).
