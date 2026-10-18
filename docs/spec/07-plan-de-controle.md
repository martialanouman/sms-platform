# 7. Plan de contrôle

La configuration, sa distribution aux services, l'Admin API qui la modifie, et qui a le droit de quoi.

## 7.1 Modèle de configuration

Postgres, schéma `control_plane`. **La base est la seule source de configuration métier** : aucun
service n'en lit dans son environnement, qui ne porte que l'infrastructure (garde AST, décision 7).

| Domaine | Tables |
|---|---|
| Clients | `customer_groups`, `customers` (facturation, `balance_scope`, `charge_on`, `content_storage`, `content_retention_days`, `max_age_override`), `rate_plans` |
| Comptes | `accounts` (débits, `max_sessions`, canaux, `query_sm`/`cancel_sm`), `credentials` (une clé d'API et un bind par compte, hachés), `webhooks` (secret scellé) |
| Expéditeurs | `sender_ids` (catégorie, statut), `sender_id_rewrite_rules`, `rate_limits` (sender ID, compte) |
| Sortie | `connectors`, `connector_accounts` (identifiants scellés, débit, binds, partition) |
| Routage | `routes`, `route_targets`, `routing_scripts`, `exact_routes` |
| Conformité | `antispam_rules`, `suppressions`, `opt_out_keywords` |
| Entrée | `inbound_numbers`, `inbound_keywords` |
| Plateforme | `platform_policy` (une ligne : politique de contenu par défaut, `quiet_hours` nullable, `billing_enabled`) |
| Contenu | `content_keys` (clé enveloppée par client, `active` / `retired` / `destroyed`) |
| Exécution | `jobs` (imports, effacement de contenu, extension de partitions), `message_export_jobs`, `gdpr_erase_jobs` |

Les secrets rejoués (bind sortant, clé d'API d'un fournisseur, secret de webhook) sont **scellés** par
`content-key-svc` (ADR-0016) et ouverts à l'usage, jamais gardés dans un instantané partagé. Un secret
n'est montré qu'une fois, à la création ou à la rotation (I2).

## 7.2 Distribution

Un seul mécanisme : **`LISTEN/NOTIFY` Postgres, plus une resync** (décision 7).

1. Le trigger d'audit de chaque table auditée appelle `pg_notify('config', '<table>:<pk>')`. Postgres
   ne délivre qu'au commit. Un import de masse émet une seule notification `<table>:*`.
2. Chaque service charge au démarrage les tables qu'il déclare lire (sa déclaration de GRANT, décision
   6), puis écoute sur une **connexion directe et dédiée** (pas par PgBouncer).
3. Sur une notification, il relit la ligne et échange atomiquement un nouvel **instantané immuable**.
   Une notification ne porte jamais de valeur.
4. **Resync** complète à chaque reconnexion de l'écoute, puis toutes les 5 min.
5. Les opt-outs ont leur canal, `optout` : le routeur ajoute la clé à son Bloom sans reconstruction.

**Hors instantané** : `exact_routes` (cache read-through sur `redis-cache`, ADR-0015 ; le Bloom se
recharge sur `exact_routes:*`) et les secrets scellés.

**Fraîcheur** : propagation p99 < 5 s, mesurée et alertée. Postgres injoignable : l'instantané courant
sert, la resync reprend au retour.

## 7.3 Admin API (`admin-api-svc`)

- **Contrat d'abord** : `contracts/openapi-admin.yaml` (OpenAPI 3.1) est déclaré **avant** l'implémentation,
  versionné par son paquet ; une rupture est un bump majeur. Le contrat que sert le code (huma) est
  comparé au fichier par un test bloquant.
- **Seul écrivain** de la configuration (hors STOP par MO et secours). Chaque mutation est une
  transaction qui pose `SET LOCAL app.actor` et `app.request_id`.
- **Seul appelant admis** : le BFF (`dashboard`). Le navigateur ne l'atteint jamais (I14).
- **Pagination** par curseur (keyset), jamais par offset.
- **Opérations longues** (import en masse, export CDR, effacement de contenu, effacement RGPD, extension de
  partitions) :
  `202` et un job dont on lit l'état.
- **Flux temps réel** `/admin/stream/*` (WebSocket) : instantanés agrégés de trafic, de sessions et de
  connecteurs, sous `stream:read`.
- **Rejeu** : `replay-dead-letter` (`dead-letters:write`) republie un record de dead-letter sur son topic
  source, `replay_count + 1`, `origin_ts` inchangé ; un record expiré ou un message déjà terminal
  (`IsTerminal`) est refusé.
- **Erreurs** : un code stable par cause, le même sur REST, SMPP et Admin (chapitre 9).

## 7.4 Autorisation

**La passerelle décide, seule** (I12). Elle n'authentifie aucun humain : elle vérifie la signature du
jeton par le JWKS interne du BFF (ADR-0019), puis le scope de l'opération.

- **Scopes** : un tag du contrat donne `<tag>:read` à ses lectures et `<tag>:write` à ses écritures
  (`routes:read`, `connectors:write`…). Plus dix scopes sensibles : `content:read`, `content:write` (stockage et rétention des corps, client et
  plateforme), `content:erase`, `gdpr:erase`,
  `msisdn:reveal`, `cdr:export_bulk`, `audit:read`, et trois qu'un rôle par défaut sépare de `:write`
  (chapitre 8 §8.5) : `routing-scripts:publish`, `suppressions:delete` (lever un opt-out),
  `billing:topup` (recharger, ajuster, fixer les conditions de crédit : mode, découvert, limite,
  activation de la facturation, d'un client ou de la plateforme). Plus `stream:read`. Ni joker ni hiérarchie. Un scope qu'aucune opération ne déclare
  n'existe pas, sauf `msisdn:reveal`.
- **Chaque opération déclare exactement un scope.** La liste est générée depuis le contrat et
  consommée par la passerelle, l'éditeur de rôles du BFF et les types de la SPA.
- **Jetons** : 5 min, émis par le BFF au nom de l'opérateur connecté ; un `kid` inconnu est refusé.
  Aucun compte de service, hors l'identité système du hub, sous `stream:read` seul (chapitre 8 §8.6).

Gardes bloquantes : chaque opération a un scope ; chaque opération appelée avec tous les scopes sauf le
sien rend 403 ; le proxy du BFF n'expose que les chemins du contrat ; le BFF n'implémente aucune
opération de la passerelle.

## 7.5 Audit

**La passerelle est la source unique de l'audit du plan de contrôle** (I13).

| Table | Contenu | Écrit par |
|---|---|---|
| `audit_log` | Un événement par mutation et par lecture sous scope sensible, y compris toute réponse démasquée par `msisdn:reveal` ; écrit une fois, à l'issue de la décision, avant la réponse : `operator` (`sub`, `service:<nom>` ou `breakglass:<nom>`), `operation_id`, `target`, `request_id`, `status`, `outcome`, `at` | Middleware de l'Admin API |
| `audit_changes` | `(request_id, actor, table, pk, op, before, after, at)` | Trigger `AFTER INSERT/UPDATE/DELETE`, dans la transaction de la mutation |

- **Acteur obligatoire** : sans `app.actor`, le trigger lève une erreur et l'écriture échoue.
- **Hors diff** : `*_sealed`, `*_hash`, `*_kms_key_ref`, `wrapped_key`.
- **Lecture d'un corps** : une ligne `granted`, `unreadable` ou `not_found`, écrite **avant** la
  réponse (I1). `content_access_audit` disparaît dans `audit_log`.
- **Classement** : chaque table de `control_plane` est auditée (diff), en événement seul
  (`exact_routes` pour le volume ; `suppressions`, `unrouted_mo` pour le MSISDN), ou exemptée avec une
  raison (grand livre, état d'exécution, journaux eux-mêmes). La garde refuse une table non classée.
- **Immuables**, partitionnées par mois, plancher de rétention de `8760 hours` tenu en base (ADR-0018).

## 7.6 Secours

BFF tombé ou clé révoquée : identifiants **dynamiques** OpenBao du rôle `breakglass`, valables une
heure, émission auditée par OpenBao ; écriture SQL sous `SET LOCAL app.actor = 'breakglass:<nom>'` ; le
trigger trace et notifie. Les validations de l'Admin API ne s'appliquent pas : c'est le prix d'un
secours sans second émetteur de jetons. Procédure au runbook.

## 7.7 Pannes

| Panne | Effet |
|---|---|
| Écriture d'audit | La mutation échoue avec elle |
| JWKS injoignable | Clés en cache ; `kid` inconnu → 401 |
| BFF arrêté | Dashboard indisponible ; plan de données intact (I15) ; secours §7.6 |
| Postgres `control_plane` | Admin API indisponible ; instantanés servis aux services |
| Connexion d'écoute | Reconnexion, puis resync complète |
