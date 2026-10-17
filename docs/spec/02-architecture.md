# 2. Architecture

Services, magasins, confiance interne, portabilité. Le détail de chaque flux vit dans son chapitre ;
celui-ci dit qui existe, ce que chacun possède et qui peut l'appeler.

## 2.1 Vue d'ensemble

```
 clients SMPP ─▶ smpp-server ─┐                           ┌─▶ connector-pool ─▶ opérateurs SMPP / HTTP
 clients REST ─▶ rest-api ────┼─▶ mt.inbound.<cat> ─▶ router ─▶ mt.routed.<cat> ┘        │
                              │                      │  ▲                                  │ dlr.events
                       session-manager        billing ─┘ (enveloppes)                      │ mo.inbound
                                                                                           ▼
          message-state ◀── mt.* , dlr.events, mo.* ──▶ dlr.routed ─┐      router (MO) ─▶ mo.routed
                │ message.state ─▶ CDR (ClickHouse)                  └──▶ delivery ◀────────┘
                                                                           │ webhook / bind SMPP
 opérateurs ─▶ navigateur ─▶ dashboard (BFF) ─▶ admin-api ─▶ control_plane  ▼ clients
```

Kafka porte tout le plan de données. Postgres porte la configuration, le crédit et l'état du message.
Aucun service du plan de données n'appelle le dashboard (I15).

## 2.2 Services

| Service | Rôle | Consomme | Produit | Possède | Appelants admis |
|---|---|---|---|---|---|
| `rest-api-svc` | Entrée REST : auth, admission, ACK durable | — | `mt.inbound.<cat>` | — | public (Ingress) |
| `smpp-server-svc` | Entrée SMPP : bind, admission, ACK durable ; remise MO/DLR sur bind | — | `mt.inbound.<cat>` | — | public (LB L4) ; `delivery-svc` |
| `session-manager-svc` | Sessions SMPP, `max_sessions` (I6) | — | — | `redis-state` (sessions) | `smpp-server-svc`, `delivery-svc`, `admin-api-svc` |
| `router-svc` | Lane MT : conformité, route, segmentation, crédit (I3) ; routage des MO | `mt.inbound.<cat>`, `mt.billing-wait.<cat>`, `mo.inbound` | `mt.routed.<cat>`, `mt.outcome`, `mo.routed` | — | aucun (Kafka seul) |
| `billing-svc` | Seule autorité du solde : enveloppes, règlement, grand livre (I4, I5). ≥ 2 répliques | `mt.routed.<cat>`, `mt.outcome` | — | tables de facturation | `router-svc`, `admin-api-svc` |
| `connector-pool-svc` | Comptes opérateur sous bail (I8), envoi SMPP et HTTP, ingress `/callbacks/{connector_account_id}` | `mt.routed.<cat>` | `mt.outcome`, `mt.reroute-park`, `dlr.events`, `mo.inbound`, `connector.account-status` | baux, curseurs de polling | public (callbacks des fournisseurs HTTP) |
| `message-state-svc` | Seul écrivain de l'état (I7), corrélation des DLR, projection CDR | `mt.*`, `dlr.events`, `mo.*` | `dlr.routed`, `dlr.uncorrelated`, `dlr.unmatched`, `message.state` | base `message_state` ; CDR ClickHouse | `rest-api-svc`, `smpp-server-svc`, `admin-api-svc` |
| `delivery-svc` | Remise des MO et DLR au client, isolée par tenant (I11) | `dlr.routed`, `mo.routed` | — | garage par tenant | aucun (Kafka seul) |
| `content-key-svc` | Clés de contenu par client, crypto-shred | — | — | clés scellées ; appelle OpenBao Transit | `message-state-svc`, `admin-api-svc` |
| `admin-api-svc` | Plan de contrôle : seul point d'autorisation (I12), audit (I13), flux temps réel | — | — | `control_plane` (écritures) | `dashboard` seul |
| `dashboard` | BFF Go et SPA : authentifie, accorde, relaie `/api/gw/*` à l'aveugle (I14) | — | — | base `dashboard` | public (navigateur) ; `admin-api-svc` (JWKS interne) |

**Jobs** : `migrate` (goose, Postgres et ClickHouse, rôle `migrator`) ; `catalog-apply` (topics, ACL
Kafka, GRANT, NetworkPolicy, depuis le catalogue et les déclarations). Ils remplacent `kafka-provision`
et toute création à la main.

**Supprimés** : `config-sync` (décision 7), `mt-replay` (devenu l'opération Admin
`replay-dead-letter`, décision 5), les 33 relais et `fakegateway` du BFF.

Les outils de test et de charge (`fake-smsc`, `load-stub`, `smpp-bindgen`, `smsc-ceiling`,
`e2e-budget`, `test-env`) ne sont pas des services : ils ne sont jamais déployés en production.

## 2.3 Magasins

| Magasin | Contenu | Écrivains | Panne |
|---|---|---|---|
| Postgres `control_plane` | Configuration, facturation, `audit_log`, `audit_changes`, `lane_parking` des services qui en ont un | `admin-api-svc` (config), `billing-svc` (crédit), chaque service pour ses propres tables | Snapshot courant servi ; envoi suspendu après 15 s (baux, I8) ; crédit selon les enveloppes en cours |
| Postgres `message_state` | Journal des transitions, `dlr_correlation`, 72 h | `message-state-svc` seul | L'envoi continue (D7, décision 2) ; lectures d'état dégradées vers le CDR |
| Postgres `dashboard` | Opérateurs, rôles, sessions, audit d'accès | `dashboard` | Le dashboard tombe ; le plan de données non (I15) |
| Kafka | Plan de données, catalogue `contracts/topics.yaml` | Selon `producers` | Admission refusée avant l'ACK (I9) |
| `redis-state` | Sessions, seaux d'admission, anti-spam, annulation, idempotence, réassemblage MO ; TTL ≤ 24 h | Par service, ACL par espace de clés | Selon la politique de chaque usage (décision 7 §5) |
| `redis-cache` | `exactroute:{msisdn}` | `router-svc` | Lecture directe par clé primaire |
| ClickHouse | CDR, historique, analytique | `message-state-svc` (projection) | Projection en retard, rattrapée depuis `message.state` |
| Stockage objet S3 | Archives CDR (Parquet) | Job d'archivage | Archivage en retard |
| OpenBao | KEK (Transit), identifiants dynamiques `breakglass` | — | Ouverture de contenu et de secrets refusée |

`message_state` vit dans sa propre base, avec son propre rôle ; le banc de la décision 2 décide s'il
lui faut un cluster dédié. Le pool de connexions passe par PgBouncer, sauf la connexion d'écoute de
configuration, directe (décision 7).

## 2.4 Le corps du message

Le corps est la seule donnée dont la fuite engage la plateforme (I1). Il vit en clair aux seuls
endroits du traitement, chacun borné dans le temps :

| Lieu | Protection | Durée |
|---|---|---|
| Topics `body: clear_bounded` (`mt.inbound.*`, `mt.billing-wait.*`, `mt.routed.*`, `mt.reroute-park`, `mo.inbound`, `mo.routed`, leurs dead-letters) | mTLS, ACL par service, chiffrement disque des brokers | Rétention ≤ 96 h : âge maximal le plus long (72 h) plus 24 h de marge |
| `lane_parking` | GRANT par service, chiffrement disque | Purgé à `max_age` |
| `redis-state` (réassemblage MO) | ACL, réseau | TTL |
| CDR ClickHouse et archives | **Scellé** par message (HKDF depuis la clé du client) | Rétention du client |

**Effacement RGPD** : le crypto-shred de la clé du client rend illisible tout le stockage durable à
l'instant. Les copies en clair dans Kafka et dans les garages disparaissent d'elles-mêmes, au plus
96 h après ; l'attestation d'effacement le déclare. Une garde du catalogue refuse un topic
`clear_bounded` dont la rétention dépasse 96 h.

## 2.5 Confiance interne

Résumé de la décision 6 ; chaque règle a un test bloquant.

- **Une CA interne**, émise par cert-manager, signe tout : services, BFF, Jobs.
- **Chaque serveur nomme ses appelants** (colonne « appelants admis » du §2.2). Une allowlist vide
  fait échouer le démarrage en production.
- **Une identité par service sur chaque magasin** : rôle Postgres, principal Kafka (CN du certificat),
  utilisateur Redis. Les droits sont générés depuis les déclarations ; un droit non déclaré est refusé.
  Le rôle `migrator` possède les schémas ; aucun service ne les possède.
- **NetworkPolicy** *deny* par défaut, flux générés depuis les allowlists et le catalogue.
- **Un Secret par service.** La KEK vit dans OpenBao Transit ; seul `content-key-svc` peut l'appeler.
- **Jetons** : le BFF les émet (ADR-0019), son JWKS est servi sur un écouteur interne réservé à
  `admin-api-svc`. Le hub temps réel ne porte que `stream:read`. Aucun compte de service.
- **Secours** : SQL sous identifiants dynamiques OpenBao d'une heure, acteur `breakglass:<nom>` (I13).

## 2.6 Portabilité

La plateforme se déploie sur Kubernetes autohébergé comme sur un cloud managé. Le code ne connaît
aucun fournisseur ; seule la configuration change.

| Brique | Autohébergé | Cloud managé |
|---|---|---|
| Postgres | Opérateur Postgres | RDS, Cloud SQL |
| Kafka (mTLS, ACL) | Strimzi | MSK, Confluent |
| Redis ×2 (ACL) | StatefulSet | ElastiCache, Memorystore |
| ClickHouse | Opérateur ClickHouse | ClickHouse Cloud |
| Stockage objet | RustFS | S3, GCS (API S3) |
| KMS | OpenBao, déverrouillage Shamir | OpenBao, déverrouillage par la KMS du fournisseur |
| Certificats | cert-manager | cert-manager |
| Réseau | NetworkPolicy | NetworkPolicy |

Une brique qui n'existe pas à l'identique des deux côtés n'entre pas. Un adaptateur vers une KMS
native ne s'écrit que lorsqu'un déploiement l'exige.
