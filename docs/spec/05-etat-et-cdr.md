# 5. État du message et CDR

Deux magasins, deux horizons : l'**état** est exact et vit 72 h dans Postgres ; le **CDR** est
l'historique et vit dans ClickHouse. Un seul service écrit les deux (I7).

## 5.1 État (`message-state-svc`)

Détail : décision 2.

- **Journal en ajout seul** `message_transitions`, base `message_state`, clé primaire
  `(message_id, segment_seq, attempt, kind)`, `ON CONFLICT DO NOTHING`. Partitionné par plage de
  `message_id` (UUIDv7, donc par jour) ; une partition de plus de 72 h est supprimée.
- **Sources** : `mt.inbound.*` (`accepted`), `mt.routed.*` (`routed`), `mt.outcome` (rejets, garage,
  issues d'envoi), `dlr.events` (après corrélation), `mo.inbound`, `mo.routed`, `mo.outcome`.
- **Écriture** par lots, par partition (20 ms ou 5 000 lignes) ; offsets commités après le commit
  Postgres.
- **Repli à la lecture**, fonction pure : par segment, dernière `attempt`, premier état terminal par
  `occurred_at`, collant ; par message, les règles publiées de `MessageStatus`. Indépendant de l'ordre.
- **`attrs`** : `connector_id`, `smsc_msg_id`, `error_code`, statut DLR brut, adresse émise après
  réécriture ; `source_addr`, `dest_addr`, `ingress` et `category` sur `accepted` et `received` ;
  `route_id`, `credits` et `charge_on` sur `routed`, pour que l'état replié, donc `message.state`, porte tout ce
  que le CDR projette. **Jamais le corps** (I1).

### Lectures

| Opération | Appelants | Cible |
|---|---|---|
| `GetMessage(message_id, account_id)` | `GET /messages/{id}`, `query_sm`, fiche message Admin | < 50 ms p99 |
| `CheckCancellable(message_id, account_id)` | `cancel_sm` | |
| `IsTerminal(message_id)` | Garde de `replay-dead-letter` | |

- **Juste après l'ACK** : un message absent du journal fait attendre, 2 s au plus, que l'offset commité
  du groupe dépasse la position de sa partition dans `mt.inbound.<cat>`. Toujours absent : CDR, puis
  404 / `ESME_RINVMSGID`.
- **Au-delà de 72 h** : le CDR répond, en tant qu'historique.
- Le message d'un autre compte reçoit la réponse d'un message inconnu.

## 5.2 CDR

### Projection

`message-state-svc` publie l'état replié de chaque message modifié sur `message.state` (clé
`message_id`, version = nombre de transitions). Une lane du même service le projette dans ClickHouse :
c'est le **seul écrivain** du CDR. Le rang de cycle de vie et ses écrivains multiples disparaissent.

| Table | Contenu | Moteur | Rétention |
|---|---|---|---|
| `cdr` | Une ligne par message et par segment : comptes, adresses, connecteur, route, catégorie, canal d'entrée, statut, codes, latence, crédits | `ReplacingMergeTree(version)`, partition par jour de `submitted_at` | 90 j par défaut, configurable |
| `cdr_content` | `message_id`, `customer_id`, corps scellé, `content_key_id`, `expires_at` | `MergeTree`, `TTL expires_at` | `content_retention_days` du client |

Le corps est dans sa propre table parce qu'il s'écrit une fois et vit moins longtemps que les
métadonnées, alors qu'une ligne de `cdr` se réécrit à chaque version de l'état.

### Le corps

- **Politique** par client, `customers.content_storage` : `off` (jamais persisté) ou
  `stored_encrypted`.
- **Scellement** : la même lane lit le corps sur `mt.inbound.*` et `mo.inbound`, dérive une clé par
  message (HKDF depuis la clé du client, que `content-key-svc` ouvre) et écrit `cdr_content`. Le clair ne
  quitte pas la mémoire.
- **Lecture** : uniquement depuis le dashboard, sous `content:read`, par l'Admin API qui demande
  l'ouverture à `content-key-svc` ; la ligne d'audit est écrite **avant** la réponse (I1). Une clé
  détruite rend 404.

### Archivage

Un Job quotidien exporte chaque jour révolu de `cdr` en Parquet vers le stockage objet, immuable,
relisible sans la plateforme. Le corps n'est jamais archivé.

| Palier | Fenêtre | Support |
|---|---|---|
| Chaud | 0–90 j | ClickHouse |
| Archive | > 90 j, jusqu'à la limite légale | Stockage objet, Parquet |

## 5.3 Rétention

| Classe | Défaut | Purge |
|---|---|---|
| État du message | 72 h | Suppression de partition |
| Corps (`cdr_content`) | `content_retention_days`, 7 j | TTL ClickHouse ; crypto-shred |
| Corps en transit (Kafka, garages) | ≤ 96 h | Rétention des topics ; `max_age` des garages (chapitre 2 §2.4) |
| CDR | 90 j | Suppression de partition (TTL) |
| Archives CDR | Politique de l'exploitant | Hors plateforme |
| Grand livre | 13 mois et plus | Chapitre 6 |
| `audit_log`, `audit_changes` | 365 j, plancher en base, 7 ans au plus | `DELETE` sous réglage transactionnel (ADR-0018) |
| Traces, logs | Jours | Jamais de corps (I1) |
| Opt-out | Sans expiration | Jamais |

## 5.4 Effacement RGPD

Sous `gdpr:erase`, job asynchrone, audité, irréversible, conclu par une **attestation**.

- **Un client** : crypto-shred de sa clé de contenu (tout `cdr_content` illisible à l'instant), puis
  suppression de ses lignes `cdr`. Le grand livre est conservé (obligation comptable).
- **Une personne (MSISDN)** : la clé est partagée entre destinataires, donc pas de crypto-shred.
  Suppression ciblée dans `cdr` (`source_addr` ou `dest_addr`) et des `cdr_content` des messages
  concernés, tous clients confondus. Vérification après mutation avant d'attester.

**Ce qui survit** (ADR-0018, amendé) :

| Quoi | Durée | Base |
|---|---|---|
| `audit_log`, `audit_changes` | Leur rétention | Imputabilité ; immuables |
| Opt-out | Sans expiration | L'effacer ré-exposerait la personne |
| Corps et MSISDN en transit (Kafka, garages) | ≤ 96 h après la demande | Rétention bornée des topics (§2.4) |
| Journal d'état | ≤ 72 h | Partitions purgées ; il porte les adresses (`attrs` de `accepted`, `received`) |
| Archives Parquet | Politique de l'exploitant | Hors plateforme |

L'attestation énumère ces exclusions et leur durée.

## 5.5 Pannes

| Panne | Effet |
|---|---|
| Postgres `message_state` | Envoi intact ; les événements attendent dans Kafka ; lectures sur le CDR, marquées « possiblement en retard » ; `cancel_sm` → `ESME_RSYSERR` |
| Perte du magasin d'état | Reconstruction par rejeu des topics sur 72 h (rétention ≥ 96 h) |
| ClickHouse | Projection en retard, rattrapée depuis `message.state` ; recherche et fiches historiques indisponibles |
| `content-key-svc` | Scellement en attente (`blocking`) : la projection du contenu prend du retard, jamais le clair n'est écrit |
| Segment sans issue | `outcome_missing` après 1 h, alerté |

## 5.6 Porte

Avant adoption, le banc de la décision 2 §9 : 75 000 transitions/s, 8 000 corrélations/s, suppression
de partition en charge, 30 min, commit de lot < 200 ms p99. En cas d'échec, le magasin d'état passe sur
ScyllaDB ; journal, repli et API restent identiques.
