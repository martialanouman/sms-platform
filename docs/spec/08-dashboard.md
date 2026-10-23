# 8. Tableau de bord

L'outil des opérateurs de l'agrégateur. Il **authentifie et accorde** ; la passerelle **décide**
(I12). Il n'est jamais sur le chemin du plan de données (I15).

## 8.1 Forme

- **Un binaire Go** (`dashboard`) qui embarque la SPA (`embed.FS`) et porte : session et
  authentification, émission des jetons, proxy vers l'Admin API, hub temps réel, journal d'accès.
- **SPA** React, TypeScript, Vite, TanStack Router et Query, sans rendu serveur ; pnpm.
- **Contrat BFF** (`contracts/openapi-bff.yaml`) : il ne décrit que ce que le BFF possède (authentification,
  opérateurs, rôles, sessions, journal d'accès, notifications). Les types Go et TypeScript en sont
  générés ; les types TypeScript de l'Admin API aussi, depuis son contrat. Une divergence casse la compilation.
- **DTO explicites** : une réponse est un struct déclaré ; un champ absent ne peut pas sortir (I1, I2).
- **≥ 2 instances** derrière un répartiteur avec affinité WebSocket. Base Postgres `dashboard`.

## 8.2 Proxy `/api/gw/*`

Vérifie la session, remplace le cookie par le jeton de l'opérateur, fait suivre **tel quel** à
`admin-api-svc` sous mTLS. Il ne garde rien, ne journalise rien, ne transforme rien, et refuse tout
chemin absent du contrat Admin. Le navigateur n'atteint jamais l'Admin API (I14).

## 8.3 Authentification

Email et mot de passe, puis MFA, sans fournisseur d'identité externe. Tout vit côté serveur.

- **Compte** : né sans mot de passe ; lien d'activation (72 h, usage unique) par e-mail. Même mécanisme
  pour la réinitialisation (1 h), qui retire tous les facteurs et ferme les sessions. Un jeton
  invalide, expiré ou remplacé reçoit le même 410.
- **Mot de passe** : 12 caractères au moins, majuscule, minuscule, chiffre, spécial ; anti-brute-force
  et verrouillage temporaire.
- **TOTP** : anti-rejeu par dernier pas consommé, fenêtre ±1 pas ; secret chiffré AES-256-GCM,
  l'identifiant de l'opérateur en données associées ; dix codes de récupération hachés, à usage unique.
  Rien ne se réaffiche (I2) ; le QR est dessiné par le client depuis l'URI `otpauth://`.
- **WebAuthn** : `rpID` et `origin` depuis la configuration, jamais la requête ; compteur de signature
  monotone (zéro constant admis) ; défis à usage unique liés à la session et à leur objet.
- **Remplacer ou ajouter un facteur** exige une session élevée ; le nouveau attend sa confirmation,
  l'ancien reste seul en vigueur jusque-là. Retirer le dernier facteur est refusé.
- **MFA obligatoire** pour tout rôle qui porte autre chose que des lectures ordinaires : un scope `:write`,
  un scope sensible (chapitre 7 §7.4) ou une permission du BFF. Tant que sa session n'est pas élevée, elle
  n'atteint que `/auth/*` ; ailleurs, proxy compris, `mfa_required`.

**Session** (décision 8, P7) : 15 min d'inactivité la ferment ; seul un geste de l'opérateur la
prolonge (`POST /auth/activity`), jamais une interrogation automatique. Le rôle `noc` n'a pas de délai
d'inactivité. Toute session prend fin 12 h après la connexion.

## 8.4 Jetons de la passerelle

- **Émis par le BFF** au nom de l'opérateur connecté (ADR-0019) : `sub` = identifiant de l'opérateur,
  scopes = union des scopes de ses rôles, `exp` 5 min.
- **Signature par OpenBao Transit** : la clé privée ne quitte jamais OpenBao, le BFF demande une
  signature à chaque jeton. Un pod compromis signe tant qu'il l'est, il ne peut pas voler la clé ; la
  rotation se fait dans OpenBao. Le JWKS est servi sur un écouteur interne réservé à `admin-api-svc` (décision 6).
- **Jeton personnel** (`gateway:personal_token`) : une heure au plus, les scopes de ses rôles moins
  `content:read`, `content:erase` et `gdpr:erase` ; son émission est journalisée.

## 8.5 Rôles

Un rôle est un ensemble de **scopes de la passerelle** (générés depuis le contrat Admin) et de
**permissions du BFF** (`operators:*`, `roles:*`, `sessions:*`, `gateway:personal_token`). Un opérateur
peut en détenir plusieurs. La SPA masque ce qui est interdit : confort, jamais une décision.

| Rôle | Portée |
|---|---|
| `Propriétaire` | Tout |
| `Exploitation` | Écriture : routage, numéros exacts, connecteurs, sessions SMPP, anti-spam, scripts et `routing-scripts:publish`, réécriture, numéros entrants, opt-out sans `suppressions:delete`, dead-letters, plateforme. `msisdn:reveal`, `cdr:export_bulk`. Lecture ailleurs |
| `Scripts` | `routing-scripts:write` sans `routing-scripts:publish` ; lecture du trafic |
| `Support` | Lecture : comptes, routage, connecteurs, sessions SMPP, CDR, trace, facturation. `msisdn:reveal`. Jamais le corps, ni les scripts, ni la réécriture |
| `Finance` | Facturation complète ; lecture ailleurs comme `Support`, sans `msisdn:reveal` |
| `Reporting` | `billing:read`, lecture du trafic |
| `Clientèle` | Écriture : clients, comptes, identifiants, groupes, facturation sans `billing:topup` |
| `Conformité` | Opt-out avec `suppressions:delete`, numéros entrants en lecture, `gdpr:erase`, `content:erase`, `msisdn:reveal`, `cdr:export_bulk`, lecture comptes et CDR |
| `Audit` | `audit:read` seul |
| `noc` | `stream:read` et lectures, aucune écriture |

**Hors de tout rôle par défaut sauf `Propriétaire`** : `content:read` (accordé à un opérateur nommé par un
rôle taillé), `content:write` (décider de conserver les corps et pour combien de temps), `operators:*` et `roles:*` (qui édite les rôles s'accorde tout), `sessions:*` (fermer la session d'un autre opérateur), `gateway:personal_token`.
Un test bloquant signale tout autre scope ou permission orphelin. Toute portée qui lit le trafic comprend
`stream:read` (sujets temps réel, §8.6).

## 8.6 Temps réel

Le hub de chaque instance s'abonne lui-même aux trois flux de la passerelle (`/admin/stream/metrics`,
`sessions`, `connectors`, identité système sous `stream:read`) et les sert au navigateur sur **une**
WebSocket par opérateur, multiplexée par sujet. Pas de couche pub/sub entre instances : deux ou trois
instances font six à neuf abonnements, ce que la passerelle absorbe. Mises à jour visibles en 2 à 5 s.

## 8.7 Alerting

**Toutes les alertes sont évaluées par Prometheus et Alertmanager**, hors du dashboard (I15). Les règles
sont du code, dans le dépôt, à côté des métriques qu'elles lisent ; elles se déploient avec la
plateforme.

- Le solde MO au plancher devient une métrique de `billing-svc` (nombre de propriétaires au plancher,
  sans identifiant en label) ; l'écran de facturation liste lesquels.
- Alertmanager notifie le BFF par webhook pour alimenter le **centre de notification** : affichage,
  jamais détection. Il pagine en direct, sans le dashboard.

## 8.8 Écrans

Ceux du chapitre 1 §1.2. Règles communes :

- **Le corps d'un message** ne s'affiche que sous `content:read`, après la ligne d'audit, et ne va dans
  aucun cache persisté, toast, URL ni message d'erreur (I1).
- **Un MSISDN** est masqué sauf `msisdn:reveal`.
- **Un secret** s'affiche une fois, à la création ou à la rotation (I2).
- **Un contrôle interdit** est désactivé et expliqué, jamais masqué sans raison.
- **Facturation désactivée** : l'écran dit « fonctionnalité indisponible ».
- **Audit** : deux onglets, « Administration de la passerelle » (journal de l'API d'administration, sous `audit:read`) et « Accès au
  dashboard » (journal du BFF, sous `audit:read` aussi), jamais fusionnés.
- Accessibilité WCAG 2.1 AA sur les parcours principaux ; bureau d'abord, tablette dégradée proprement.

## 8.9 Journal d'accès

Les seuls faits du BFF : connexions, déconnexions, échecs de mot de passe ou de MFA, demande et usage
d'un lien d'accès, enrôlements et retraits de facteurs, création, désactivation et réactivation d'opérateurs,
changements de rôles d'un opérateur, création, modification et suppression d'un rôle, fermeture forcée
d'une session, émission d'un jeton personnel, refus de session. Un opérateur se désactive, il ne se supprime pas : le journal tient ses auteurs.

## 8.10 Pannes

| Panne | Effet |
|---|---|
| BFF | Dashboard indisponible ; plan de données et alerting intacts ; secours SQL (chapitre 7 §7.6) |
| Admin API | Écrans en erreur explicite ; authentification et journal d'accès intacts |
| OpenBao | Aucun jeton neuf : les jetons émis valent jusqu'à leur `exp` |
| Postgres `dashboard` | Connexion impossible ; sessions perdues |
| Un flux temps réel | Reconnexion avec backoff ; l'écran signale la donnée figée |
