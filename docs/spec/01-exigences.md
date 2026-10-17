# 1. Exigences

## 1.1 Passerelle — exigences fonctionnelles

### Modèle et provisionnement

- **Client et comptes.** Un **client** (`customers`) détient un ou plusieurs **comptes**
  (`smpp_accounts`). Chaque compte a exactement un identifiant de bind SMPP et exactement une clé
  d'API. Le client porte la facturation (soldes, plan tarifaire, mode `charge_on`), les sender IDs et
  l'appartenance à un groupe ; le compte porte les plafonds de débit, `max_sessions`, les canaux et le
  webhook.
- **Provisionnement par l'exploitant seul**, par l'Admin API et le tableau de bord : clients, comptes,
  identifiants (rotation manuelle avec fenêtre de grâce), canaux, plafonds, webhooks, sender IDs,
  facturation. Aucune surface en libre-service.
- **Groupes de clients** : une dimension d'organisation (filtrage, reporting), plate ; un client
  appartient à zéro ou un groupe. Un groupe ne porte ni solde, ni quota, ni règle.

### Entrée

- **Serveur SMPP** v3.4 et 5.0 : binds des clients, `submit_sm`, `deliver_sm`, `enquire_link`,
  `unbind`, et `query_sm` / `cancel_sm` désactivables par compte.
- **API REST** minimale : soumettre un MT, consulter le statut de ses propres messages. Les MO et les
  DLR arrivent par webhook.
- **Sessions SMPP** : authentification, keep-alive, fenêtre par session, `max_sessions` par compte
  (**I6**), unbind gracieux, déconnexion forcée, anti-brute-force du bind.
- **Admission du débit avant l'ACK**, par sender ID puis par compte : au-delà, `429` /
  `ESME_RTHROTTLED`, rien n'est écrit. Jamais de refus de débit après l'ACK.
- **Quiet hours, opt-in** (décision 8, P2) : désactivées, tout part. Activées, l'exploitant définit la
  fenêtre de la plateforme, à l'heure du pays de référence ; un message `marketing` soumis dans la
  fenêtre est refusé à l'admission (`quiet_hours`).
- **ACK durable** : un message n'est accusé qu'une fois écrit dans `mt.inbound.<catégorie>`
  (`acks=all`).

### Traitement MT

- **Conformité** : normalisation E.164 (indicatif du pays de référence), **autorisation de sender ID**
  (le `source_addr` correspond à un sender ID approuvé et actif du client, numérique compris), opt-out
  bloquant, anti-spam (règles de contenu, vélocité MT et MO, doublons).
- **Routage en trois niveaux** : (0) numéro exact (portabilité), prioritaire et court-circuitant, mais
  jamais pour la conformité (**I3**) ; (1) script de routage de l'exploitant (JavaScript ou Lua, isolé,
  borné) ; (2) matching déclaratif (compte, client, sender ID, préfixe ou MCC-MNC, contenu, heure,
  priorité), avec route de repli et chaîne de repli entre connecteurs.
- **Encodage et segmentation** : GSM-7, UCS-2, 8-bit ; UDH ; surcharge d'encodage par connecteur.
- **Réservation de crédit** (si la facturation est active) : sans appel réseau tant qu'une enveloppe
  couvre le message (décision 1).
- **Réécriture de sender ID** : à portée plateforme, client, compte ou connecteur, évaluée juste avant
  l'envoi ; l'original reste sur le CDR.
- **Âge maximal** : `otp` 10 min, `transactional` 24 h, `marketing` 72 h, plafonné par la
  `validity_period` du message, surchargeable par client à la baisse. Au-delà : `expired`, jamais
  envoyé (**I10**).

### Sortie

- **Connecteurs multi-protocoles** : un connecteur est une destination (SMPP ou HTTP) qui contient N
  **comptes opérateur**, chacun avec ses identifiants, son débit contractuel (~25 SMS/s) et son plafond
  de binds (décision 3). Créer un compte depuis le tableau de bord le fait binder en quelques secondes,
  sans déploiement.
- **SMPP sortant** : binds TX/RX/TRX, fenêtre, `enquire_link`, reconnexion automatique opt-in avec
  backoff, disjoncteur **par compte opérateur**. `link_status` et `breaker_state` sont distincts.
- **HTTP sortant** : un adaptateur Go par fournisseur ; voie retour par webhook ou par polling.
- **Débit par compte opérateur**, appliqué à l'envoi : il ralentit l'envoi, il ne rejette pas.
- **Bascule** : un compte indisponible republie son backlog vers un compte frère, puis vers le
  connecteur suivant de la chaîne de repli, puis en dead-letter.

### Voie retour

- **MO** : routés vers le bon compte par numéro entrant ou mot-clé ; facturés sur un compteur distinct
  si la facturation est active ; un `STOP` inscrit l'opt-out.
- **DLR** : corrélés au message d'origine par `message-state-svc` (décision 2), transmis au compte par
  `deliver_sm` ou webhook.
- **Remise isolée par tenant** : un client injoignable ou lent ne retarde que sa propre file ; remise
  tentée **24 h** au plus depuis la réception, puis dead-letter (décision 4).
- **Opt-out** : liste de suppression scopée au canal (numéro entrant) ; mots-clés `STOP` du pays de
  référence.
- **Numéros entrants et mots-clés** : shortcodes et long codes de l'agrégateur, assignés à un compte
  (dédiés) ou résolus par mot-clé (partagés) ; file des MO non routés.

### État, contenu, conformité

- **État du message** exact et lisible dès l'ACK, pendant la phase active puis la fenêtre DLR
  (72 h au plus) ; au-delà, le CDR répond en tant qu'historique (décision 2).
- **CDR** : une ligne par message et par segment, projetée depuis l'état ; recherche, export.
- **Stockage du contenu, configurable** : défaut plateforme, surcharge par client ; s'il est stocké, il
  l'est chiffré par une clé par client, lisible seulement sous `content:read` (**I1**).
- **Effacement RGPD à la demande** : un client entier ou une personne (MSISDN), avec attestation.
- **Traçabilité** : un `trace_id` propagé à chaque étape ; le corps n'apparaît jamais dans une trace
  ni un log (**I1**).

### Facturation (optionnelle)

- **Crédits entiers**, prépayés ou postpayés, soldes MT et MO séparés, propriétaire du solde au niveau
  du client ou du compte.
- **Mode de facturation par client** (`charge_on`) : `submit` (à la réponse du SMSC, quel que soit le
  statut), `accept` (à un `submit_sm_resp` ROK, **défaut**), `delivery` (au DLR).
- **Dépassement borné** : au plus T × R par propriétaire, T = 1 s (**I5**). Facturation indisponible
  et enveloppe vide : le message est garé, jamais envoyé sans couverture au-delà de la borne.
- Désactivée, le pipeline tourne sans latence ni dépendance ajoutées.

### Plan de contrôle

- **Admin API** interne, consommée par le tableau de bord : le seul point de décision d'autorisation
  (**I12**), un scope par opération.
- **Audit** : chaque mutation et chaque lecture sensible, avec les valeurs avant et après, dans la
  transaction (**I13**).
- **Rejeu des dead-letters** : une opération Admin, auditée.
- **Métriques temps réel** pour le tableau de bord et l'alerting.

## 1.2 Tableau de bord — exigences fonctionnelles

- **Clients et comptes** : navigation à deux niveaux, création, modification, suspension (suspendre un
  client suspend ses comptes) ; au niveau client : sender IDs (catégorie, débit), facturation,
  politique de contenu, groupe ; au niveau compte : identifiant et clé d'API (création, rotation avec
  grâce, révocation, **I2**), canaux, `query_sm` / `cancel_sm`, `max_sessions`, webhook.
- **Groupes de clients** : CRUD, affectation, filtre partout où un client apparaît.
- **Connecteurs et comptes opérateur** : formulaire progressif (champs requis, puis section avancée),
  état en direct par compte opérateur (`link_status`, `breaker_state`, détenteur, lag), rebind et
  déconnexion forcée, débit et taux d'erreur.
- **Routes** : constructeur visuel des règles déclaratives (priorité, conditions, stratégie de
  distribution), routage par numéro exact, import en masse.
- **Scripts de routage** : éditeur (JS, Lua), validation, test contre un payload, versions,
  publication et retour arrière, affectation, métriques d'exécution.
- **Réécriture de sender ID** : CRUD par portée, avec test.
- **Opt-out** : listes de suppression, import en masse, outil « pourquoi ce message a-t-il été
  bloqué ? ».
- **Numéros entrants et mots-clés** : CRUD, affectation, file des MO non routés.
- **Sessions en direct** : binds clients et comptes opérateur, déconnexion forcée.
- **Trafic temps réel** : compteurs MT et MO, succès et échecs, latences, ventilation par connecteur,
  client, compte, groupe, canal.
- **CDR** : recherche paginée et filtrée, détail avec chronologie et corps si stocké et autorisé
  (**I1**), export asynchrone gouverné.
- **Trace d'un message** par `message_id` ou `trace_id` ; sans corps.
- **Anti-spam** : CRUD des règles, file de revue.
- **Facturation** : soldes MT et MO distincts, recharges, plan tarifaire, mode `charge_on`, grand
  livre. Se dégrade en « fonctionnalité indisponible » si la facturation est désactivée.
- **Contenu et RGPD** : politique de stockage, lecture de corps auditée, effacement.
- **Dead-letters** : consultation et rejeu par groupe.
- **Plateforme** : quiet hours (activation, fenêtre).
- **Alerting** : règles en code, évaluées par Prometheus et Alertmanager, indépendamment du tableau de
  bord (**I15**) ; le tableau de bord affiche le centre de notification.
- **Audit** : deux onglets, une source chacun — plan de contrôle (passerelle) et accès au tableau de
  bord (BFF) (décision 5).
- **Opérateurs et rôles** : un rôle est un ensemble de scopes de la passerelle et de permissions du
  BFF ; rôles par défaut, dont **`noc`** en lecture seule.
- **Authentification** : email et mot de passe, MFA (TOTP et WebAuthn), sans fournisseur d'identité
  externe ; jeton personnel d'une heure au plus, inclus dans les droits de l'opérateur.
- **Session** : 15 min d'inactivité la ferment, une interrogation automatique ne la prolonge pas ; le
  rôle `noc` tient 12 h sans délai d'inactivité, sans aucune écriture (décision 8, P7).

## 1.3 Exigences non fonctionnelles

### Passerelle

| Catégorie | Cible |
|---|---|
| Débit | **8 000 SMS/s** soutenus (exigence 5 000 à 10 000), **15 000** en pic. Débit **traversant** : de la soumission à l'envoi |
| Latence d'ingestion | p50 < 50 ms, p99 < 250 ms (soumission → ACK) |
| Latence bout en bout | p50 < 400 ms, p99 < 2 s (soumission → tentative SMSC), compte opérateur non saturé, disjoncteur fermé ; hors garage, backpressure et dead-letter délibérés |
| Durabilité | Aucune perte après l'ACK (**I9**) ; remise au SMSC au moins une fois. Doublons bornés et nommés (spec 03) |
| Disponibilité | 99,95 % par région. Postgres injoignable suspend l'envoi après 15 s (exclusivité des comptes opérateur, décision 3) |
| Isolation | Une panne propre à un tenant ne retarde jamais un autre tenant (**I11**) |
| Scalabilité | Horizontale et linéaire des niveaux sans état ; le pool s'étend par comptes opérateur |
| Facturation | Aucun appel réseau sur le chemin chaud tant qu'une enveloppe couvre ; surcoût nul si désactivée |
| Sécurité | TLS sur REST et SMPP, identifiants entrants hachés, secrets rejoués scellés, mTLS interne avec allowlist, une identité par service sur chaque magasin |
| Conformité | RGPD, opt-out, effacement à la demande, rétention configurable (90 j par défaut) |
| Observabilité | Métriques en moins de 5 s, logs structurés, traces ; jamais de corps (**I1**) |
| Traçage | 100 % des étapes couvertes ; 100 % des messages en échec échantillonnés |
| Configuration | Propagation p99 < 5 s (décision 7) |

### Tableau de bord

| Catégorie | Cible |
|---|---|
| Latence temps réel | Métriques et sessions visibles en 2 à 5 s |
| Opérateurs simultanés | 100 à 300 |
| Disponibilité | 99,9 %, en topologie multi-instance (au moins deux BFF) |
| Portée d'une panne | La visualisation seule ; jamais le débit ni la détection (**I15**) |
| Fraîcheur de recherche | CDR cherchable 10 à 30 s après traitement |
| Sécurité | Autorisation dans la passerelle (**I12**), MFA pour les rôles privilégiés, corps et secrets jamais exposés hors permission (**I1**, **I2**) |
| Réactivité | Bureau d'abord ; dégradation propre sur tablette |
| Accessibilité | WCAG 2.1 AA sur les parcours principaux |

## 1.4 Contraintes

- **Go** pour tous les services et le BFF ; **React** (Vite, TypeScript, TanStack Router et Query,
  pnpm) pour la SPA.
- **Un pipeline central unique** pour SMPP et REST, sans duplication de logique.
- **Kubernetes**, autohébergé ou managé, avec les mêmes briques des deux côtés (décision 6).
- **Mono-pays** : un pays de référence par plateforme (décision 8, P1).
- **Identifiants UUIDv7** : `message_id` et `trace_id` générés à l'ingestion, jamais des secrets,
  scopés par compte sur l'API publique ; tables du plan de contrôle sur le générateur natif de
  Postgres 18.
- **Les clients ne s'authentifient jamais** contre l'Admin API ni le tableau de bord.
- **Facturation opt-in**, désactivée par défaut, activable globalement et par client.
- **Reconnexion SMPP sortante opt-in** par connecteur ; le disjoncteur ne reconnecte jamais.

## 1.5 Estimation

### Trafic

- 8 000 SMS/s soutenus, 15 000 en pic ; volume quotidien réaliste 150 à 250 M messages.
- Ratio MT:MO ~80:20 ; 1,3 segment en moyenne.

### Connexions

- Binds clients simultanés : 5 000 à 20 000.
- **Comptes opérateur** : ~320 à 25 SMS/s pour 8 000 SMS/s ; ~1 000 partitions sur les trois
  `mt.routed.<catégorie>` (une par compte et par catégorie).
- Connexions REST simultanées : plus de 10 000.

### Stockage

- CDR : ~0,5 à 1 Ko par message ; à 250 M/jour × 0,8 Ko ≈ **200 Go/jour**, ~18 To sur 90 jours avant
  compression.
- **Journal d'état** : ~5 à 7 transitions par message ; **75 000 transitions/s** et 8 000 recherches de
  corrélation/s à la cible, sur 72 h. Porte d'adoption : le banc de la décision 2.
- Grand livre : jusqu'à une entrée par segment facturé ; partitionné.
- Métriques : dizaines de milliers de séries actives (compte × connecteur × route).

### Calcul

- ~1 vCPU soutient 1 000 à 2 000 msg/s de routage, anti-spam et encodage : 8 à 16 vCPU soutenus,
  2 à 3× en pic. Un compte à script de routage a une enveloppe propre, inférieure.
- Kafka dimensionné pour 8 000 à 15 000 écritures/s, réplication 3.
- Traces : 6 à 8 spans par message, jusqu'à ~120 000 spans/s en pic ; échantillonnage tête et queue,
  100 % des échecs.

### Tableau de bord

- Flux temps réel agrégé en amont : ~3 000 messages/s de diffusion pour 300 opérateurs × 10 widgets.
- Recherche CDR : dizaines par seconde, sur de larges plages.
- Sessions : des dizaines de milliers, en pagination et deltas.
