# 9. Contrats

Ce qu'un composant promet à un autre. **Le contrat se déclare avant l'implémentation**, et le code s'y
conforme, jamais l'inverse. Tous vivent sous `contracts/`.

| Fichier | Entre | Garde de compatibilité |
|---|---|---|
| `topics.yaml` | Producteurs et consommateurs Kafka | Gardes du §9.1 |
| `proto/` (payloads Kafka, gRPC interne) | Services | `buf breaking` : un record en rétention (96 h) ou un pod de la version précédente doit rester lisible |
| `openapi-public.yaml` | Clients de l'agrégateur | `oasdiff` : une rupture `ERR` exige une version majeure de l'API publique |
| `openapi-admin.yaml` | BFF et passerelle | `oasdiff` en avertissement seul : déployés ensemble (décision 9, C2) |
| `openapi-bff.yaml` | SPA et BFF | Idem, servis par le même binaire |

La liste des scopes et des permissions, les types Go du BFF et les types TypeScript de la SPA sont **générés** depuis ces
fichiers ; `check-generated` échoue sur un écart. Le paquet npm `gateway-api-contracts` et la copie de
contrat du dashboard disparaissent : il n'y a plus de second dépôt à servir.

Le contrat que servent `rest-api-svc` et `admin-api-svc` (huma) est comparé au fichier par un test
bloquant ; le BFF génère son serveur depuis le sien (oapi-codegen strict-server). Toute opération déclarée
est servie.

## 9.1 Catalogue des topics

Chaque topic et chaque groupe de consommateurs est déclaré dans `contracts/topics.yaml` (décision 4).
Un Job idempotent en applique partitions, rétention, `cleanup.policy` et ACL ; une page de
documentation en est générée.

**Topic** : `key`, `order` (`per_key`, `per_partition` pour une clé `manual`, `none`), `duplicate_unit` et où le doublon est absorbé,
`payload` (message protobuf versionné), `body` (`none`, `clear_bounded`), `partitions`, `retention`,
`compacted`, `producers`.

**Groupe** : `topic`, `service`, `lane` (facultatif, quand un service a deux lanes sur le même topic ;
le groupe se nomme `<service>[.<lane>].<topic>`), `assignment` (`group` par défaut ; `broadcast` quand
chaque pod lit tout, `manual` quand un bail assigne les partitions),
`tenant`, `dependencies` (chacune `blocking`, `park_by_tenant`,
`park_global` ou `fail_open` justifié), `concurrency` (`per_partition`, `per_tenant` plafonné),
`max_age`, `dead_letter` (obligatoire), `start` (`earliest` ; `latest` justifié).

| Topic | Clé | Ordre | Corps | Producteurs | Consommateurs |
|---|---|---|---|---|---|
| `mt.inbound.<cat>` | `message_id` | aucun | oui | rest-api, smpp-server | router, message-state |
| `mt.billing-wait.<cat>` | `message_id` | aucun | oui | router | router |
| `mt.routed.<cat>` | manuelle (compte opérateur) | par partition | oui | router, pool | pool, billing, message-state |
| `mt.reroute-park` | `message_id` | aucun | oui | pool | pool |
| `mt.outcome` | `message_id` | par clé | non | router, pool, smpp-server (`cancelled`) | message-state, billing |
| `dlr.events` | `connector_account_id` | par clé | non | pool | message-state |
| `dlr.uncorrelated`, `dlr.unmatched` | `connector_account_id` | aucun | non | message-state | message-state |
| `dlr.routed` | `account_id` | par clé | non | message-state | delivery, billing |
| `mo.inbound` | `connector_account_id` | par clé | oui | pool | router, message-state |
| `mo.routed` | `account_id` | par clé | oui | router | delivery, billing, message-state |
| `mo.outcome` | `message_id` | par clé | non | delivery, router (`unrouted`) | message-state |
| `message.state` | `message_id` | par clé | non | message-state | message-state (projection CDR) |
| `connector.account-status` | `connector_account_id`, compacté | par clé | non | pool | router |
| `<groupe>.dead-letter` | celle de la source | aucun | selon la source | runtime | `replay-dead-letter` |

**En-têtes communs** : `origin_ts` (posé par le premier producteur, jamais modifié), `trace_id`,
`replay_count`, `replayed_at`. Ce sont les seuls : toute autre donnée est un champ du payload, versionné
et gardé par `buf breaking`. Un record de dead-letter enveloppe sa source dans `sms.v1.DeadLetter` (payload et
en-têtes d'origine, raison, instant, position source) et recopie les quatre en-têtes (step 008a).

**Gardes bloquantes** :

1. Tout topic produit ou consommé dans le code a son entrée, et toute entrée a un usage.
2. Un service ne produit que là où il figure dans `producers`.
3. Une dépendance dont une lane signale l'échec ou l'absence est déclarée, dans le mode qui s'y applique :
   sinon la lane s'arrête, après avoir commité ce qui est fait. Le runtime n'agit sur un mode qu'à l'échec
   de la dépendance ; il ne fournit pas les clients (amendé à la step 008a).
4. Un topic où transite le corps est `clear_bounded`, de rétention ≤ 96 h, inscrit à l'inventaire RGPD.
5. Le nombre de partitions d'un topic `order: per_key` est figé.
6. `maxReplicas` d'un service consommant par groupe (`assignment: group`) ≤ partitions du plus petit
   topic qu'il consomme.
7. Les ACL Kafka générées correspondent exactement au catalogue.

## 9.2 gRPC interne

| Service | Opérations | Appelants |
|---|---|---|
| `billing-svc` | Octroi et clôture d'enveloppe ; recharge, transfert, ajustement, bascule de `balance_scope` ; lectures | router, admin-api |
| `message-state-svc` | `GetMessage`, `CheckCancellable`, `IsTerminal` | rest-api, smpp-server, admin-api |
| `session-manager-svc` | Ouverture et fermeture de session, `max_sessions`, recherche des binds actifs, déconnexion forcée | smpp-server, delivery, admin-api |
| `smpp-server-svc` | Remise d'un `deliver_sm` sur un bind tenu | delivery |
| `content-key-svc` | Ouverture de la clé d'un client ; provisionnement et rotation (`admin-api-svc` seul) ; scellement et ouverture des secrets (`ConfigSecrets`) ; destruction | message-state, admin-api ; tout service qui ouvre un secret de configuration |

Toujours sous mTLS, appelants nommés (décision 6).

## 9.3 Codes d'erreur

**Un code stable par cause**, le même partout : réponse REST, `command_status` SMPP, `error_code` de
l'état et du CDR. Modèle plat `{ code, message, errors[] }`. Un code a, selon ses surfaces, un statut
HTTP, un statut SMPP, ou seulement une existence dans l'état. Ajouter un code touche le catalogue Go,
l'`enum` des contrats OpenAPI concernés et ce tableau, ensemble ; un test garde la correspondance.

| Code | HTTP | SMPP | Quand |
|---|---|---|---|
| `unauthenticated` | 401 | `ESME_RINVPASWD` | Clé ou bind invalide (jamais `ESME_RINVSYSID` : il révélerait les `system_id`) ; au BFF, session absente ou identifiants refusés |
| `account_suspended`, `channel_disabled` | 403 | `ESME_RBINDFAIL` | Compte ou canal fermé |
| `max_sessions_exceeded` | — | `ESME_RBINDFAIL` | I6 |
| `rate_limited` | 429 | `ESME_RTHROTTLED` | Débit, avant l'ACK ; lecture REST ; anti-brute-force du BFF |
| `quiet_hours` | 403 | `ESME_RSUBMITFAIL` | Marketing dans la fenêtre (chapitre 3) |
| `payload_too_large` | 413 | `ESME_RINVMSGLEN` | Imports Admin ; au BFF, plafond de corps avant décodage |
| `validation_error` | 422 | `ESME_RINVMSGLEN` | Requête mal formée |
| `idempotency_conflict` | 409 | — | Clé réutilisée pour un autre corps, ou encore en cours |
| `queue_full`, `service_unavailable` | 503 | `ESME_RMSGQFUL`, `ESME_RSYSERR` | ACK impossible ; au BFF, hachage saturé ou dépendance injoignable |
| `invalid_destination`, `invalid_source` | — | — | E.164 |
| `sender_id_not_authorized` | — | — | |
| `recipient_opted_out` | — | — | |
| `content_blocked` | — | — | Anti-spam |
| `no_route` | — | — | |
| `insufficient_credit` | 402 | — | Transfert de solde (Admin) ; en MT, après l'ACK, un état |
| `billing_unavailable` | — | — | Garage de facturation expiré (chapitre 6) ; remplace `external_billing_unavailable` |
| `message_expired` | — | — | `max_age` dépassé (I10) |
| `submit_failed` | — | — | Refus de l'opérateur |
| `ambiguous_submit` | — | — | Délai dépassé après émission HTTP, sans idempotence |
| `incomplete_message` | — | — | Segments manquants à l'adaptateur HTTP |
| `fallback_exhausted`, `retries_exhausted` | — | — | Repli ou retentatives épuisés |
| `delivery_failed`, `delivery_expired` | — | — | DLR d'échec |
| `message_not_found` | 404 | `ESME_RINVMSGID` | Inconnu ou d'un autre compte |
| `cancel_failed` | 409 | `ESME_RCANCELFAIL` | Déjà parti (ADR-0013) |
| `operation_not_supported` | 405 | `ESME_RINVCMDID` | `query_sm`/`cancel_sm` désactivé |
| `not_found`, `conflict`, `forbidden_scope` | 404, 409, 403 | — | Admin API ; `not_found` et `forbidden_scope` aussi au BFF |
| `forbidden_origin`, `mfa_required` | 403 | — | BFF : origine étrangère ; session non élevée d'un opérateur qui porte une écriture, un scope sensible ou une permission du BFF |
| `unsupported_media_type` | 415 | — | BFF : méthode non sûre sans `application/json` |
| `access_link_invalid` | 410 | — | BFF : lien d'accès servi, expiré, remplacé, ou vers un compte désactivé |
| `password_policy`, `mfa_code_refused`, `webauthn_ceremony_refused` | 422 | — | BFF : contenu refusé sur une requête bien formée |
| `mfa_elevation_required`, `mfa_already_enrolled`, `mfa_replacement_refused`, `mfa_nothing_to_confirm`, `mfa_last_factor` | 409 | — | BFF : état des facteurs |
| `email_taken`, `role_name_taken`, `self_lockout`, `operator_disabled`, `role_is_default`, `role_held` | 409 | — | BFF : opérateurs et rôles |
| `internal_error` | 500 | `ESME_RSYSERR` | |

Les codes après l'ACK ne sont jamais une réponse synchrone : ils sont lus par `GET /messages/{id}` ou
dans le CDR ; seuls `delivery_failed` et `delivery_expired` arrivent aussi par l'accusé.
