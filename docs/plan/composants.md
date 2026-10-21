# Plan des composants

Les onze services sur le socle, jusqu'à la fin du chapitre 1. Le numéro continue celui du socle et reste
l'ordre d'exécution (C6). Chaque composant est porté de `go-gateway` ou `go-gateway-bo`, ou réécrit,
selon ce que la spec en exige ; la step le dit dans ses arbitrages.

**Découpage mixte** (décision humaine du 2026-10-19). Les fondations et le chemin de données se découpent
par composant. Les capacités d'exploitation se découpent en **tranches verticales** : la règle côté
passerelle, l'opération Admin et l'écran dans la même step, parce qu'un écran n'est que la face visible
d'une opération Admin (I14, spec 08 §8.1) et ne se prouve pas contre une API absente. Un écran de lecture
arrive avec les données qu'il lit. Les coupes en a/b sont arrêtées dès le plan (Fable, 2026-10-19) ;
une autre peut s'ajouter au démarrage d'une step, comme au socle.

| Step | Livre | Phase |
|---|---|---|
| 008a | Runtime de lane : lane `group`, dead-letter, gardes | Fondations |
| 008b | Runtime de lane : garage global, `per_tenant` | Fondations |
| 009 | Instantané de configuration | Fondations |
| 010a | Brique OpenBao | Fondations |
| 010b-1 | `content-key-svc` : cœur et adaptateurs | Fondations |
| 010b-2 | `content-key-svc` : service et déploiement | Fondations |
| 011a-1 | Prometheus et Alertmanager | Fondations |
| 011a-2a | Dashboard : schéma et GRANT par base | Fondations |
| 011a-2b | Dashboard : cœur et adaptateurs | Fondations |
| 011a-3a | Dashboard : base sur le cluster, migrations multi-base, Pooler | Fondations |
| 011a-3b | Dashboard : service et déploiement | Fondations |
| 011a-4a | Dashboard : SPA, chaîne et embarquement | Fondations |
| 011a-4b | Dashboard : écrans de connexion et de lien d'accès | Fondations |
| 011b-1 | Dashboard : TOTP, codes de récupération, session élevée | Fondations |
| 011b-2a | Dashboard : charte graphique | Fondations |
| 011b-2b | Dashboard : écrans de vérification et d'enrôlement | Fondations |
| 011b-3 | Dashboard : WebAuthn | Fondations |
| 012a | Dashboard : rôles et autorisation | Fondations |
| 012b | Dashboard : opérations opérateurs et rôles | Fondations |
| 012c-1 | Dashboard : émetteur, Transit, jeton personnel | Fondations |
| 012c-2 | Dashboard : écouteur interne du JWKS et déploiement | Fondations |
| 012d | Dashboard : coquille et écran Opérateurs | Fondations |
| 012e | Dashboard : écrans Rôles et jeton personnel | Fondations |
| 013 | `admin-api-svc` : autorisation, audit, proxy ; écran Audit | Fondations |
| 014a | Clients, groupes, comptes | Tranche |
| 014b | Identifiants, webhooks | Tranche |
| 015 | Plateforme, sender IDs, débits | Tranche |
| 016 | Connecteurs, comptes opérateur, réécriture | Tranche |
| 017 | Routage déclaratif et numéros exacts | Tranche |
| 018 | Scripts de routage | Tranche |
| 019 | Opt-out et anti-spam | Tranche |
| 020 | Numéros entrants et mots-clés | Tranche |
| 021 | `rest-api-svc` | Chemin MT |
| 022 | `router-svc` : lane MT | Chemin MT |
| 023a | `connector-pool-svc` : baux, statut, SMPP sortant | Chemin MT |
| 023b | `connector-pool-svc` : ordonnancement, HTTP, reroute, réécriture | Chemin MT |
| 024 | `message-state-svc` : état et CDR | Chemin MT |
| 025a | `session-manager-svc` | Chemin MT |
| 025b | `smpp-server-svc` | Chemin MT |
| 026 | Temps réel et centre de notification | Tranche |
| 027 | CDR, trace, archivage, dead-letters | Tranche |
| 028 | Voie retour : DLR | Voie retour |
| 029a | Voie retour : réception des MO au pool | Voie retour |
| 029b | Voie retour : lane MO, MO non routés | Voie retour |
| 029c | Runtime de lane : garage par tenant | Voie retour |
| 030 | `delivery-svc` | Voie retour |
| 031a | Facturation : enveloppes, règlement | Tranche |
| 031b | Facturation : grand livre, écran | Tranche |
| 032 | Contenu et RGPD | Tranche |

## Ce que chaque step porte

Des couplages du `CLAUDE.md`, appliqués ici à chaque fois ; une step ne les rappelle pas.

- **Un service neuf** : son `cmd/`, son build GoReleaser, son manifest `deploy/k8s/`, ses flux de
  NetworkPolicy, sa déclaration de GRANT, les règles d'alerte des métriques qu'il émet (spec 10 §10.1),
  son PodDisruptionBudget s'il en a un (spec 10 §10.2). Il est **terminé quand la release le déploie
  sur l'environnement de test** et que son `/readyz` y répond ; puis son paquet GHCR devient public.
- **Une brique de l'environnement de test entre avec son premier consommateur** : PgBouncer (Pooler
  CloudNativePG), Mailpit pour les e-mails, Prometheus et Alertmanager en 011a (les règles de 007c y
  trouvent leur évaluateur) ; `redis-state` en 021 ; `redis-cache` en 022 ; `fake-smsc` en 023a ;
  ClickHouse en 024 ; stockage objet en 027.
- **Une table** : sa migration, son classement d'audit, ses GRANT ; un canal de notification s'il est
  distribué (spec 07 §7.2).
- **Une opération Admin** : déjà au contrat 1.0.0 ; sinon le contrat change d'abord. Le test « tous les
  scopes sauf le sien » la couvre dès son arrivée.
- **Un écran** : types TypeScript générés, contrôles interdits désactivés et expliqués, masquage du
  MSISDN, secrets montrés une fois (I2), WCAG 2.1 AA sur le parcours.
- **Les dettes du territoire** : la step liste les issues `debt` ouvertes qui le touchent et dit, pour
  chacune, soldée ou reportée.

## 008 — Runtime de lane

Le runtime unique de la décision 4 (D6), sous `internal/lane` : classification des échecs, garage
par tenant (table `lane_parking` du service) ou global (topic), expiration depuis `origin_ts` (I10),
dead-letter, rejeu, commits. Les gardes du catalogue qui lisent le code (1 à 3, 6) entrent ici.

- **008a** : lane `group`, dépendances `blocking` et `fail_open`, âge, dead-letter, producteur, gardes.
- **008b** : `park_global`, `per_tenant`. Le garage par tenant passe en 029c, avec son consommateur
  (humain, 2026-10-19).

**Terminé quand** : un poison part en dead-letter sans bloquer sa partition ; un client lent ne retarde
que sa file sous la fenêtre non commitée du pod ; chaque garde est rouge sur un consommateur fautif.

## 009 — Instantané de configuration

La bibliothèque de la décision 7 : chargement des tables déclarées au démarrage, écoute `LISTEN/NOTIFY`
sur une connexion directe, relecture de la ligne et échange atomique d'un instantané immuable, resync à
la reconnexion puis toutes les 5 min. Le `Cuttable` de Postgres (005e) et la métrique de fraîcheur, avec
son alerte p99 > 5 s.

**Terminé quand** : une écriture sous `app.actor` arrive dans l'instantané d'un lecteur ; une coupure de
l'écoute est suivie d'une resync complète ; une notification `<table>:*` recharge la table.

## 010 — OpenBao et `content-key-svc`

- **010a, brique OpenBao** dans l'environnement de test : Transit, rôle `CREATEROLE` `openbao` par les
  rôles gérés de CloudNativePG (report de 007d-1), `breakglass` créé sous `openbao`, qui seul peut en
  accorder l'adhésion (Fable, 2026-10-19).
- **010b, `content-key-svc`** : scellement et ouverture des secrets rejoués sous la clé maître (ADR-0016) ;
  seul appelant d'OpenBao Transit. Les clés par client et le crypto-shred viennent en 032. Premier
  service : la garde de `deploy.sh` (ordre des Jobs et des services) y est revue. Coupée au `cmd/` (Fable,
  2026-10-20).

**Terminé quand** : un secret scellé ne s'ouvre que par `content-key-svc` ; seuls ses appelants admis
l'atteignent.

## 011 — Dashboard : BFF et authentification

- **011a** : `cmd/dashboard`, base `dashboard` et ses migrations, la SPA embarquée (`embed.FS`) et sa
  chaîne (pnpm, Vite) dans la CI unique ; comptes nés sans mot de passe, liens d'activation et de
  réinitialisation, politique de mot de passe, anti-brute-force, sessions (15 min, 12 h,
  `POST /auth/activity`), journal d'accès ; écrans de connexion, d'activation et de réinitialisation. Prometheus
  ouvre l'entrée 8200 d'OpenBao à sa collecte (010a).
  Coupée en quatre (Fable, 2026-10-20) : Prometheus et Alertmanager ; cœur et adaptateurs ; service et
  déploiement ; SPA et écrans (`docs/steps/011a-1-prometheus-et-alertmanager.md`).
  011a-2 coupée en deux (humain, 2026-10-20) : schéma et GRANT par base ; cœur et adaptateurs
  (`docs/steps/011a-2a-schema-par-base.md`).
  011a-3 coupée en deux (humain, 2026-10-20) : base, migrations multi-base et Pooler ; service et déploiement
  (`docs/steps/011a-3a-base-dashboard-et-pooler.md`).
  011a-4 coupée en deux (humain, 2026-10-20) : chaîne et embarquement ; écrans (`docs/steps/011a-4a-chaine-spa.md`).
- **011b** : TOTP, codes de récupération, WebAuthn, session élevée, remplacement d'un facteur ; écrans
  d'enrôlement.
  Coupée en trois (humain, 2026-10-20) : TOTP, codes et session élevée côté serveur ; écrans de vérification et
  d'enrôlement ; WebAuthn, serveur et écran (`docs/steps/011b-1-totp-et-elevation.md`).
  011b-2 coupée en deux (humain, 2026-10-20) : la charte graphique de l'origine, puis les écrans
  (`docs/steps/011b-2a-charte.md`).

**Terminé quand** : un opérateur s'active, se connecte, enrôle un facteur et voit sa session close à
l'inactivité ; chaque fait du §8.9 a sa ligne au journal.

## 012 — Dashboard : opérateurs, rôles, jetons

Opérateurs et rôles (scopes générés et permissions du BFF, rôles par défaut du §8.5, test des orphelins)
et leurs écrans, MFA obligatoire selon la portée, émission des jetons signés par OpenBao Transit, JWKS
sur l'écouteur interne, jeton personnel.
Coupée en cinq (Fable, 2026-10-21) : rôles et autorisation ; opérations opérateurs et rôles ; jetons ; coquille et écran
Opérateurs ; écrans Rôles et jeton personnel (`docs/steps/012a-roles-et-autorisation.md`).
012c coupée en deux (Fable, 2026-10-21) : cœur et adaptateurs ; écouteur interne et déploiement
(`docs/steps/012c-1-emetteur-et-transit.md`).

**Terminé quand** : un jeton porte l'union des scopes des rôles de l'opérateur et se vérifie par le JWKS
interne ; une session non élevée d'un rôle qui écrit n'atteint que `/auth/*`.

## 013 — `admin-api-svc` : autorisation, audit, proxy

- **Passerelle** : vérification du jeton par le JWKS du BFF (`kid` inconnu refusé), scope par opération
  (I12), middleware d'audit (I13), contrat servi comparé au fichier : sous-ensemble jusqu'en 032,
  égalité stricte ensuite.
- **BFF** : proxy `/api/gw/*` aveugle, limité aux chemins du contrat (I14).
- **Écran Audit** : deux onglets, plan de contrôle et accès au dashboard, première tranche bout en bout.

**Terminé quand** : chaque opération appelée avec tous les scopes sauf le sien rend 403 ; un chemin hors
contrat est refusé par le proxy ; l'écran Audit lit les deux journaux sous `audit:read`.

## 014 à 020 — Tranches de configuration

Chacune livre ses tables, ses opérations Admin et ses écrans. Le cœur de la règle qu'elle configure
(`internal/<ctx>/`, sans I/O) n'y entre que si une opération Admin de la tranche l'exerce ; sinon il
arrive avec son service (017 → 022, 020 → 029b).

- **014a — Clients, groupes, comptes** : navigation à deux niveaux, suspension en cascade, canaux,
  `max_sessions`, `query_sm` / `cancel_sm`.
- **014b — Identifiants, webhooks** : clé d'API et bind hachés (rotation avec grâce, révocation, I2),
  webhooks au secret scellé.
- **015 — Plateforme, sender IDs, débits** : `platform_policy` (quiet hours, politique de contenu,
  `billing_enabled`), sender IDs et catégories, `rate_limits`.
- **016 — Connecteurs, comptes opérateur, réécriture** : formulaire progressif, identifiants scellés,
  débit et binds, extension de partition (job) ; règles de réécriture à leurs quatre portées et
  `test-sender-rewrite-rule`. L'état en direct vient en 026.
- **017 — Routage déclaratif et numéros exacts** : routes, cibles, route de repli, `exact_routes` et son
  cache, import en masse (`jobs`, `202`).
- **018 — Scripts de routage** : éditeur, validation, `test-routing-script` (goja, mur horaire),
  versions, publication et retour arrière, affectation. Les métriques d'exécution viennent en 026.
- **019 — Opt-out et anti-spam** : suppressions et mots-clés, canal `optout`, import,
  `check-suppression`, règles anti-spam. La file de revue n'a pas d'opération au contrat 1.0.0 (debt
  #45) : le contrat change d'abord, ou elle reste reportée.
- **020 — Numéros entrants et mots-clés** : CRUD et affectation ; la file des MO non routés vient en
  029b.

**Terminé quand** (chacune) : un opérateur au bon rôle configure la capacité depuis son écran, la
mutation est auditée avec son acteur, et un lecteur de l'instantané la voit.

## 021 à 025 — Chemin MT

- **021 — `rest-api-svc`** : clé d'API, seaux `redis-state` (sender ID puis compte, fail-closed), quiet
  hours, idempotence, ACK durable sur `mt.inbound.<cat>` (spec 03 §3.2).
- **022 — `router-svc`** : la lane MT sur le runtime de 008, assemblant les cœurs de 015 et de 017 à 019
  (I3), segmentation, publication sur `mt.routed.<cat>`, `assignment: broadcast` du runtime, lag par lane et ses règles ; réservation de crédit en no-op jusqu'en 031a.
  Sans statut de compte opérateur, tout message finit `no_route` : le test injecte un record de
  `connector.account-status`.
- **023a — `connector-pool-svc`** : `assignment: manual` et `fail_open` du runtime, baux (I8), `connector.account-status`, seau, jeton d'annulation,
  SMPP sortant contre `fake-smsc`.
- **023b** : ordonnancement par catégorie, un premier adaptateur HTTP, reroute et `fallback_chain`,
  réécriture de sender ID (cœur de 016).
- **024 — `message-state-svc`** : journal, repli, `GetMessage`, `CheckCancellable`, `IsTerminal`,
  projection CDR dans ClickHouse ; base
  `message_state`, câblage goose de ClickHouse ; `GET /messages/{id}` de `rest-api-svc`. `load-stub`
  entre ici pour le banc §5.6.
- **025a — `session-manager-svc`** : sessions sur `redis-state`, `max_sessions` (I6).
- **025b — `smpp-server-svc`** : bind, `submit_sm`, `query_sm`, `cancel_sm`.

**Terminé quand** : un message soumis en REST puis en SMPP atteint `fake-smsc` sur l'environnement de
test, et son état se lit ; un compte opérateur sans détenteur laisse ses messages dans leur partition
(I9) ; le banc de la spec 05 §5.6 passe (75 000 transitions/s, 8 000 corrélations/s).

## 026 — Temps réel et centre de notification

`/admin/stream/*` dans la passerelle, hub du BFF (une WebSocket par opérateur), écrans Trafic, Sessions
en direct, état des connecteurs (rebind, déconnexion forcée), métriques d'exécution des scripts ;
webhook d'Alertmanager vers le centre de notification.

**Terminé quand** : un envoi sur l'environnement de test apparaît à l'écran Trafic en moins de 5 s ; une
alerte qui part s'affiche au centre de notification, et pagine sans le dashboard (I15).

## 027 — CDR, trace, archivage, dead-letters

Recherche paginée du CDR et sa fiche, trace d'un message, Job d'archivage Parquet (spec 05 §5.2), export
asynchrone gouverné (stockage objet), consultation et rejeu des dead-letters (`replay-dead-letter`).

**Terminé quand** : un message envoyé se retrouve au CDR et à sa trace sans corps ; un jour de CDR est
archivé ; un rejeu repasse par toutes les étapes, et un message expiré ou terminal est refusé.

## 028 à 030 — Voie retour

- **028 — DLR** : normalisation par le pool, corrélation par `message-state-svc`, `dlr.uncorrelated` et
  `dlr.unmatched`, fenêtre DLR.
- **029a — Réception des MO** : par le pool (bind, callbacks, polling), acquittement après production
  durable.
- **029b — Lane MO** : réassemblage, STOP, résolution du compte (cœur de 020), file des MO non routés à
  l'écran.
- **029c — Garage par tenant** : `park_by_tenant` du runtime de lane (`lane_parking`, disjoncteur, sonde,
  vidange sous `SKIP LOCKED`), conçu contre le traitement de `delivery-svc` ; le critère I11 se ferme ici :
  une dépendance propre à un tenant gare ce tenant et laisse passer les autres.
- **030 — `delivery-svc`** : remise par webhook et par bind, garage par tenant (I11), dead-letter à
  24 h.

**Terminé quand** : un DLR et un MO de `fake-smsc` arrivent au webhook d'un compte ; un endpoint client
injoignable n'en retarde aucun autre.

## 031 — Facturation

Placée après la voie retour : elle règle depuis `mt.routed.<cat>`, `mt.outcome`, `dlr.routed` et
`mo.routed`, et elle est optionnelle. Désactivée, `billing-svc` n'est pas déployé (spec 10 §10.2) :
l'environnement de test l'active.

- **031a** : `billing-svc`, enveloppes, réservation du routeur, règlement des trois `charge_on`, solde
  MO.
- **031b** : grand livre, écran Facturation (soldes, recharges, plan tarifaire, « fonctionnalité
  indisponible »).

**Terminé quand** : I4 et I5 tiennent sous double livraison, rejeu et reroute ; facturation injoignable,
le routeur gare et n'envoie rien sans crédit ; le banc de la spec 06 §6.7 passe.

## 032 — Contenu et RGPD

`content_keys` (clé enveloppée par client, `rotate-content-key`), politique de stockage des corps,
lane de `message-state-svc` qui les scelle au CDR (`cdr_content`, spec 05 §5.2), lecture auditée sous `content:read` (I1), effacement de contenu et effacement RGPD (crypto-shred,
attestation).

**Terminé quand** : un corps ne s'affiche qu'après sa ligne d'audit ; après effacement, aucun stockage
durable ne le rend lisible.

## Après les composants

La mise en production : la checklist de la spec 10, les issues `go-live`, et la fin du dépôt public.
