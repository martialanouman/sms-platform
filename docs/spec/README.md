# Plateforme SMS — spécification

La spécification unique du monorepo : la passerelle SMS et son tableau de bord d'exploitation. Elle
remplace les deux specs d'origine (`go-gateway`, `go-gateway-bo`). Elle a été construite depuis le
code, les ADR et les neuf décisions de `docs/design/`, jamais depuis le guide d'ingénierie, qui
diverge du code (décision 9, C8).

## Ce qu'on construit

Une **passerelle SMS** pour un agrégateur national : elle reçoit des SMS (SMPP entrant et REST), les
route vers des opérateurs (SMPP ou HTTP sortants) et gère la voie retour (MO, DLR). Les deux protocoles
d'entrée partagent **un pipeline unique**. Cible : **8 000 SMS/s** soutenus, **15 000** en pic.

Un **tableau de bord d'exploitation** la pilote : un BFF Go et une SPA React. Il authentifie les
opérateurs et leur accorde des droits ; la passerelle seule décide ce que chaque requête peut faire.

Les clients de l'agrégateur n'ont **aucun accès** à la plateforme hormis l'envoi, la réception et le
statut de leurs propres messages. Tout se provisionne par l'exploitant.

## Chapitres

| Fichier | Contenu |
|---|---|
| [01-exigences](01-exigences.md) | Exigences fonctionnelles et non fonctionnelles, contraintes, estimation |
| [02-architecture](02-architecture.md) | Services, magasins, confiance interne, portabilité |
| [03-pipeline-mt](03-pipeline-mt.md) | Ordre du pipeline, admission, routage, crédit, comptes opérateur, lanes |
| [04-voie-retour](04-voie-retour.md) | Pipeline MO, corrélation des DLR, remise isolée par tenant |
| 05-etat-et-cdr | État du message, projection CDR, rétention, RGPD |
| 06-facturation | Enveloppes de crédit, `charge_on`, grand livre |
| 07-plan-de-controle | Modèle de configuration, distribution, Admin API, autorisation et audit |
| 08-dashboard | BFF, authentification, rôles, écrans |
| 09-contrats | Catalogue des topics, contrats d'API, codes d'erreur |
| 10-exploitation | Observabilité, déploiement, secours, mise en production |
| [decisions](../design/) | L'audit et les neuf designs qui fondent cette spec |

## Non-objectifs

Chacun est tranché ; aucun n'est un report.

| Non-objectif | Pourquoi |
|---|---|
| **Campagnes** : listes, destinataires, modèles, suivi côté client | La plateforme reçoit, traite et route ; composer une campagne appartient à l'application cliente |
| **Envoi programmé ou différé** | La passerelle envoie quand elle reçoit ; le seul délai est opérationnel (file, retry, backpressure, `validity_period`) |
| **Portail ou API en libre-service** pour les clients | Onboarding B2B, provisionné par l'exploitant |
| **Règles réglementaires d'un autre pays de destination** | Mono-pays (décision 8, P1). Envoyer vers l'étranger reste possible par le routage |
| **Réputation anti-spam** | Lue, jamais écrite ; l'anti-spam se compose de règles et de compteurs (P3) |
| **Plafond journalier** `max_per_day` | Le crédit borne la dépense, l'admission borne le débit (P4) |
| **Fournisseur de facturation externe** | Seul le grand livre interne existe (P5) |
| **API de collecte des MO et des DLR** | Réception par webhook ou par bind SMPP (P6) |
| **Gabarit générique de connecteur HTTP** en base | Un adaptateur Go par fournisseur (décision 3) |
| **Équité de volume entre tenants** | On promet l'isolation des pannes ; l'admission borne le volume (décision 4) |
| **Reprise après sinistre inter-région** (RPO, RTO, bascule) | Relève de l'exploitation de l'infrastructure |

## Invariants

Numérotation unique (décision 9, C4). Le code les cite par leur numéro ; chacun a un test bloquant, vert
à vie.

| # | Invariant | Origine |
|---|---|---|
| **I1** | **Le corps d'un message ne sort jamais de l'écran qui l'affiche** : ni log, ni trace, ni toast, ni URL, ni message d'erreur, ni export, ni cache persisté, ni journal d'état. L'affichage exige `content:read` et une ligne d'audit écrite avant la réponse | passerelle (a), dashboard (a) |
| **I2** | **Un secret n'est jamais réaffiché.** Identifiants, clés d'API, secrets de webhook : montrés une fois, à la création ou à la rotation. Aucune action « révéler » | dashboard (b) |
| **I3** | **Un message routé par numéro exact traverse toutes les étapes de conformité** (sender ID, opt-out, anti-spam). Le court-circuit saute la résolution de route, jamais la conformité | passerelle (b) |
| **I4** | **Un segment est facturé au plus une fois**, sous double livraison, rejeu ou reroute : clé primaire `(message_id, segment_seq)` | passerelle (c), décision 1 |
| **I5** | **Conservation du crédit** : pour toute enveloppe réglée, `granted = Σ crédits consommés non libérés + refunded`, et `SUM(grand livre) == solde`. Le dépassement d'un propriétaire reste ≤ T × R | décision 1 |
| **I6** | **`max_sessions` refuse le bind au-delà du quota** ; côté sortant, un compte opérateur ne dépasse ni `max_binds` ni `throughput_limit_per_sec` | passerelle (d), décision 3 |
| **I7** | **Un seul écrivain de l'état du message**, `message-state-svc`. Le repli ne dépend pas de l'ordre ; le premier état terminal reste | décision 2 |
| **I8** | **Au plus un détenteur actif par compte opérateur** | décision 3 |
| **I9** | **Aucun message n'est perdu après l'ACK.** Il part, ou il atteint un état terminal (`rejected`, `expired`, `not_sent`…), ou il attend durablement (partition, garage, dead-letter). Vaut pour un compte sans détenteur et pour une dépendance en panne | spec §1.2, décisions 3 et 4 |
| **I10** | **Aucun message expiré n'est envoyé.** L'âge se mesure depuis `origin_ts`, à travers garages, dead-letters et rejeux | décision 4 |
| **I11** | **Une lane ne bloque jamais sur une dépendance propre à un tenant** | décision 4 |
| **I12** | **L'autorisation se décide dans la passerelle** : chaque opération Admin a exactement un scope, appliqué quel que soit le chemin d'entrée | dashboard (c), décision 5 |
| **I13** | **Toute écriture du plan de contrôle est attribuée et tracée** dans sa transaction ; une écriture sans acteur échoue | décision 5 |
| **I14** | **Le navigateur n'atteint jamais l'Admin API en direct** : le BFF relaie, la clé de signature, le mTLS et Postgres ne quittent jamais le serveur | dashboard (d) |
| **I15** | **Le tableau de bord n'est jamais sur le chemin du plan de données** : sa panne dégrade la visualisation, jamais le débit ni la détection d'incident | dashboard (e) |

## Glossaire

| Terme | Sens |
|---|---|
| **Client** (`customer`) | L'entreprise cliente de l'agrégateur. Porte la facturation, les sender IDs, le groupe |
| **Compte** (`account`, `account_id`) | Un compte SMPP d'un client : un identifiant de bind et une clé d'API. Porte les débits, les sessions, les webhooks. À l'écran, « compte » |
| **Connecteur** | Une destination sortante (un opérateur, un agrégateur), cible des routes. Protocole `smpp` ou `http` |
| **Compte opérateur** (`connector_account_id`) | Un compte contractualisé chez l'opérateur, dans un connecteur : identifiants, débit (~25 SMS/s), binds. Jamais `account_id` |
| **Tenant** | Ce qui isole une file d'un autre : un compte client sur la voie retour, un compte opérateur au pool |
| **Lane** | Un consommateur Kafka et sa partition, sous le contrat du catalogue (décision 4) |
| **Garage** | Le lieu où un message attend le retour d'une dépendance, par tenant (table) ou global (topic) |
| **Enveloppe** (`CreditLease`) | Crédit pré-débité, détenu par un pod du routeur pour un propriétaire (décision 1). Distinct de l'enveloppe du message (`pipeline.Envelope`) |
| **Segment** | Une partie UDH d'un SMS concaténé ; l'unité de facturation et de `mt.routed` |
| **Catégorie** | `otp`, `transactional` ou `marketing`, portée par le sender ID ; fixe le topic, la priorité et l'âge maximal |
| **Opérateur** | Un humain du tableau de bord. À ne pas confondre avec l'opérateur télécom, que la spec appelle « opérateur » seulement dans « compte opérateur » et « SMSC opérateur » |
