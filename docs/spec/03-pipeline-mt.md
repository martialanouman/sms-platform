# 3. Pipeline MT

Le chemin d'un message de la soumission à l'opérateur. Deux entrées (REST, SMPP), un seul pipeline.

## 3.1 Ordre

**Non réordonnable.** Chaque étape suppose les précédentes faites.

| # | Étape | Service | Refus |
|---|---|---|---|
| 1 | Authentification (clé d'API ou bind) | ingestion | `401` / `ESME_RINVPASWD` |
| 2 | Débit : sender ID, puis compte | ingestion | `429` / `ESME_RTHROTTLED` |
| 3 | Choix du topic (lecture de la catégorie) et quiet hours | ingestion | `quiet_hours` |
| 4 | **ACK durable** sur `mt.inbound.<cat>` | ingestion | `503` / `ESME_RSYSERR` si Kafka refuse |
| 5 | Normalisation E.164 | routeur | `rejected` |
| 6 | Autorisation du sender ID | routeur | `rejected` |
| 7 | Opt-out | routeur | `rejected` |
| 8 | Anti-spam | routeur | `rejected` |
| 9 | Résolution de route : numéro exact → script → déclaratif ; choix du compte opérateur | routeur | `rejected` (`no_route`) |
| 10 | Encodage et segmentation | routeur | `rejected` |
| 11 | Réservation de crédit | routeur | `rejected` (crédit), ou garage |
| 12 | Débit du compte opérateur, jeton d'annulation | pool | — (ralentit, ne rejette pas) |
| 13 | Envoi (`submit_sm` ou HTTP) | pool | `submit_failed`, `not_sent` |
| 14 | Capture ou libération du crédit | facturation | — |
| 15 | État et CDR | message-state | — |

- **Avant l'ACK, rien n'est écrit** : un refus est une réponse synchrone. **Après l'ACK, un refus est un
  état** (`rejected`, publié sur `mt.outcome`) : le client le lit par DLR ou par `GET /messages/{id}`.
- **Le débit se refuse avant l'ACK, jamais après.** Au-delà, le plafond du compte opérateur ralentit
  l'envoi.
- **Le court-circuit « numéro exact » saute l'étape 9, jamais les étapes 5 à 8** (I3).

## 3.2 Ingestion (`rest-api-svc`, `smpp-server-svc`)

1. **Authentification.** REST : clé d'API hachée. SMPP : le bind authentifie la session, `max_sessions`
   la borne (I6), l'anti-brute-force la protège.
2. **Débit.** Seaux `redis-state` atomiques, sender ID puis compte (ADR-0021). En panne de Redis :
   plafond local par pod, fail-closed.
3. **Catégorie et quiet hours.** La catégorie du `source_addr` se **lit** dans l'instantané de
   `sender_ids` ; un expéditeur inconnu part en `marketing` et le routeur le rejettera. Si les quiet
   hours sont actives (`platform_policy`) et qu'un message `marketing` tombe dans la fenêtre, à l'heure
   du pays de référence : refus `quiet_hours`, jamais différé.
4. **Idempotence REST** : une clé `Idempotency-Key` rejouée sous 24 h rend la réponse d'origine.
5. **ACK.** Production `acks=all` sur `mt.inbound.<cat>`, clé `message_id` (UUIDv7, attribué ici).
   L'en-tête `origin_ts` est posé à cet instant et ne change plus (I10). Puis `202` / `submit_sm_resp`.

Le corps est lu ici pour la longueur et l'encodage déclaré, jamais journalisé (I1).

## 3.3 Lane du routeur (`router-svc`)

Un consommateur par `mt.inbound.<cat>`, lane du catalogue (décision 4). Chaque record :

1. **Âge.** Plus vieux que son `max_age` (§3.6) : `expired`, terminal, non facturé.
2. **E.164**, avec l'indicatif du pays de référence.
3. **Sender ID.** Le `source_addr` soumis, avant réécriture, doit être un sender ID `active` du client,
   numérique compris (ADR-0020). Le routeur **recalcule la catégorie** depuis son instantané ; si un
   instantané d'ingestion en retard a laissé passer un `marketing` dans la fenêtre des quiet hours, le
   routeur le rejette (`quiet_hours`). C'est la seule voie d'un refus de quiet hours après l'ACK.
4. **Opt-out.** Bloom en mémoire, alimenté au fil de l'eau par le canal `optout` (décision 7) ; un
   positif est confirmé dans Postgres, dépendance `blocking`. Opt-out scopé au canal (ADR-0007).
5. **Anti-spam.** Règles de contenu, `category_mismatch` (ADR-0020), compteurs de vélocité MT et MO,
   doublons. Pas de réputation. Les règles tournent en mémoire sur le corps transitoire ; seuls
   l'identifiant de règle et le verdict en sortent (I1).
6. **Route.**
   - **L0, numéro exact** : `exactroute:{msisdn}` sur `redis-cache`, en read-through (ADR-0015) ; en
     panne, lecture par clé primaire.
   - **L1, script** (goja), borné par un mur horaire.
   - **L2, déclaratif** : compte, client, sender ID, préfixe ou MCC-MNC, contenu, heure, priorité ;
     route de repli.
   - Filtre `priority_tier` (ADR-0020) aux trois niveaux ; un connecteur filtré compte comme
     indisponible. Rien ne reste : `no_route`.
   - **Choix du compte opérateur** : hachage rendez-vous pondéré par `throughput_limit_per_sec`, sur
     `message_id`, parmi les comptes disponibles d'après `connector.account-status` (décision 3). Un
     connecteur sans compte disponible cède au suivant de la route.
   - **Priorité effective** = `min(max(demandée, défaut), plafond)` de la catégorie (ADR-0020).
7. **Encodage et segmentation** : GSM-7, UCS-2 ou 8-bit, surcharge par connecteur ; UDH. Tous les
   segments partent sur le même compte.
8. **Crédit** (si la facturation est active). Décrément de l'enveloppe du propriétaire en mémoire
   (décision 1). Crédit insuffisant : `rejected`. Facturation injoignable et enveloppe vide : le
   message va sur `mt.billing-wait.<cat>` (`park_global`), jamais envoyé sans crédit.
9. **Publication** : un record par segment sur `mt.routed.<cat>`, partition du compte opérateur. Il
   porte `lease_id`, crédits, `charge_on`, version du plan, catégorie, priorité effective.

Les rejets partent sur `mt.outcome`, `segment_seq = 0`. Aucune dépendance propre à un tenant ne peut
bloquer une partition du routeur (I11).

## 3.4 Pool (`connector-pool-svc`)

Le détenteur du bail d'un compte opérateur (I8) consomme ses trois partitions (une par catégorie) :

1. **Âge** : expiré, `not_sent` (`expired`), jamais envoyé (I10).
2. **Ordonnancement** au sein du compte : OTP → transactionnel → marketing, le marketing garde au moins
   `marketing_min_share` (0,4) des envois tant qu'il attend (ADR-0021).
3. **Seau local** à `throughput_limit_per_sec` ; une file pleine bloque la consommation, le message
   reste dans Kafka.
4. **Bind** : `hash(message_id) % binds_actifs` ; les segments d'un message sur le même bind.
5. **Réécriture de sender ID** (plateforme, client, compte, connecteur).
6. **Jeton d'annulation** `SET cancel:{id} dispatched NX GET` (ADR-0013) : `cancel` lu, le message
   n'est pas envoyé (`not_sent`). Seule dépendance `fail_open` de la plateforme.
7. **Envoi** : `submit_sm`, ou l'adaptateur HTTP, qui réassemble les segments (décision 3 §6).
8. **Issue** sur `mt.outcome` : `submitted` (avec `remote_id`), `submit_failed`, `not_sent`.

Le disjoncteur est par compte. Un compte en panne avec backlog republie vers un compte frère, puis vers
le connecteur suivant de `fallback_chain`, par `mt.reroute-park` ; chaîne épuisée : dead-letter.

## 3.5 Doublons

Un abonné peut recevoir deux fois le même SMS. C'est assumé et borné, jamais facturé deux fois (I4) :

| Frontière | Cause | Borne |
|---|---|---|
| Routeur (ADR-0014) | Crash entre production sur `mt.routed` et commit de l'offset | Un poll par partition et par incident |
| Pool (ADR-0012) | Crash entre `submit_sm` et production de l'issue | `FetchMaxPartitionBytes` 56 KiB compressés, ~250 SMS par partition et par crash |
| Pool, bail | Envois en vol au moment de la perte du bail | Fenêtre d'auto-fencing (TTL − 3 s) |

Le rejeu d'une dead-letter repasse par toutes les étapes ; la clé `(message_id, segment_seq)` absorbe
la facturation.

## 3.6 Âge maximal

| Catégorie | `max_age` |
|---|---|
| `otp` | 10 min |
| `transactional` | 24 h |
| `marketing` | 72 h |

Plafonné par la `validity_period` du message ; surchargeable par client, à la baisse seulement. Mesuré
depuis `origin_ts`, à travers garages, reroutes, dead-letters et rejeux. Un message expiré n'est pas
rejouable.

## 3.7 Annulation

Réservée à SMPP (`cancel_sm`, ADR-0009). `message-state-svc` répond inconnu, déjà soumis ou annulable ;
l'annulation se joue sur le jeton `SET cancel:{id} cancel NX GET` : vainqueur, `cancelled` ; perdant
(`dispatched`), `ESME_RCANCELFAIL` et aucune ligne. `cancelled` signifie « jamais parti ».

## 3.8 Pannes

| Panne | Effet |
|---|---|
| Kafka refuse l'écriture | Pas d'ACK : `503` / `ESME_RSYSERR`. Le client réessaie |
| `redis-state` | Admission : plafond local fail-closed. Annulation : fail-open, compté |
| `redis-cache` | L0 par clé primaire Postgres |
| Postgres `control_plane` | Instantanés servis ; confirmation d'opt-out `blocking` ; baux perdus en 15 s, envoi suspendu (I8 prime) |
| `billing-svc` | Enveloppes en cours consommées, puis garage `mt.billing-wait.<cat>` jusqu'au retour ou à `max_age` |
| `message-state-svc` | Aucun effet sur l'envoi ; l'état rattrape |
| Compte opérateur | Republication vers un frère, puis repli ; sans détenteur, les messages attendent dans leur partition (I9) |
| Pod du routeur ou du pool | Rééquilibrage ou bail réclamé ; doublons bornés (§3.5) |
