# Step 010a — Brique OpenBao

Plan : `docs/plan/composants.md` §010, première des deux PR. Spec : `docs/spec/02-architecture.md` §2.3, §2.5,
§2.6 ; `docs/spec/07-plan-de-controle.md` §7.6 ; `docs/spec/10-exploitation.md` §10.1, §10.3, §10.4. Décision :
`docs/design/06-confiance-interne.md` §3, §6, §8. ADR : 0011, 0016. Précédents : `docs/steps/007d-1-briques.md`,
`docs/steps/007b-2-grants-en-production.md`. Règles : `.claude/rules/ci-deploy.md`, `.claude/rules/tests.md`.
Mode : PR.

## Contexte

- Aucune brique KMS : la KEK de la décision 6 n'a pas de domicile ; `content-key-svc` (010b) en a besoin.
- 007d-1 a renvoyé ici les rôles `openbao` et `breakglass`, « créés sous un autre rôle que `migrator` ».
- Le générateur de GRANT ne touche que les rôles dont `migrator` a l'`ADMIN OPTION` : un rôle né hors de lui
  lui échappe.
- Aucune table `control_plane` de configuration avant 014a ; Prometheus n'entre qu'en 011a.
- Dettes du territoire : #108 (PITR de `control_plane`) ne touche pas OpenBao, reste ouverte.

## Arbitrages

Fable, 2026-10-19, sauf le point 3 tranché par l'humain.

1. **Coupe** : 010a (brique), 010b (`content-key-svc`). 010b se teste contre un conteneur OpenBao et ne
   dépend de 010a que pour sa preuve sur le VPS.
2. **Une clé Transit par domaine** (`config-secrets` ici, `content` en 032) : la politique n'ouvre que
   `encrypt` et `decrypt` de cette clé ; elle remplace le tag de 32 octets d'ADR-0016, qui séparait deux
   domaines d'un espace de clé partagé. `aes256-gcm96`, ni exportable ni supprimable (défauts).
3. **`LocalKMS` n'est pas porté** (humain, 2026-10-19) : l'adaptateur se teste contre OpenBao ; ADR-0011 et
   06 §11 s'amendent en 010b.
4. **Authentification Kubernetes** : rôle `content-key-svc` lié au SA `sms/content-key-svc`, audience
   `openbao`, jeton d'une heure, politique `content-key-svc` seule (pas de `default`). Le SA d'OpenBao porte
   `system:auth-delegator` (chart). Le rôle est inerte jusqu'au SA de 010b.
5. **`breakglass` naît sous `openbao`** : CloudNativePG ne sait pas poser l'`ADMIN OPTION`, que PG 16+
   exige pour créer un rôle `IN ROLE breakglass`. `managed.roles` crée `openbao` (`LOGIN CREATEROLE`) ;
   `breakglass` (`NOLOGIN`) est créé sous `SET ROLE openbao`, qui en reçoit l'`ADMIN OPTION`. OpenBao n'en
   fait jamais tourner le mot de passe (`rotate-root`) : CloudNativePG le réconcilierait.
6. **Droits de `breakglass` sur les tables** : pas d'`ALTER DEFAULT PRIVILEGES` (il couvrirait `audit_log`,
   immuable, 07 §7.5) ; une déclaration générée entre avec la première table que le secours écrit. Issue
   `debt`.
7. **Brique** : chart `openbao` 0.30.2 (OpenBao v2.7.1), namespace `openbao`, raft à une réplique (le
   standalone n'a pas de snapshot, 10 §10.3), listener TLS sous la CA interne, télémétrie sans
   authentification, injecteur et UI coupés, PDB coupé (une réplique : il bloquerait tout drain ; 10 §10.2
   n'en demande pas).
8. **Init et configuration à la main** : `bao operator init`, parts Shamir et jeton racine hors dépôt ;
   descellement manuel (10 §10.4). `deploy/bricks/openbao.sh`, idempotent, joué par l'humain : le jeton
   racine et le mot de passe d'`openbao` passent par l'entrée standard, jamais par un argument.
   `deploy.sh` attend le pod `Running`, pas le descellement : un timer bloqué sur un OpenBao scellé
   retiendrait toutes les releases. Amendé sur le VPS (humain, 2026-10-20) : le script ne révoque plus le
   jeton, l'humain le révoque son travail fini. Depuis OpenBao 2.5.3, `generate-root` exige un jeton
   (`disable_unauthed_generate_root_endpoints`, vrai par défaut) : révoquer le dernier fermait OpenBao à
   toute administration. Secours d'un jeton perdu : rouvrir l'endpoint sur le listener, redémarrer,
   desceller, `generate-root`, refermer.
9. **Alerte** `OpenBaoSealed` : `vault_core_unsealed == 0`, et `absent` pour un OpenBao arrêté ou
   injoignable. Elle entre avec la brique qui émet la métrique. Amendé sur le VPS : scellé, OpenBao sert
   ses métriques (200, `vault_core_unsealed{cluster=""} 0`) ; Fable le supposait muet.
10. **Dettes** : droits de `breakglass` (#119, 014a) ; authentification humaine à OpenBao pour lire
    `database/creds/breakglass` (#120, `go-live`) ; snapshot chiffré d'OpenBao (#121, 10 §10.3, `go-live`).
    #108 : reportée, hors territoire.

## Design arrêté

- **`deploy/bricks/namespaces.yaml`** : namespace `openbao`.
- **`deploy/bricks/openbao.yaml`** : le `HelmChart` `openbao` (repo `https://openbao.github.io/openbao-helm`,
  chart 0.30.2, cible `openbao`) et la `NetworkPolicy` d'entrée d'OpenBao : 8200 depuis les pods
  `app.kubernetes.io/name: content-key-svc` de `sms`.
- **`deploy/bricks/ca.yaml`** : `Certificate` `openbao` (namespace `openbao`, `server auth`, un an,
  `openbao.openbao.svc` et `127.0.0.1`) ; son `ca.crt` vérifie aussi Postgres.
- **`deploy/bricks/postgres.yaml`** : rôle géré `openbao` (`LOGIN CREATEROLE`, Secret `openbao`) ; entrée
  5432 depuis les pods OpenBao.
- **`deploy/bricks/secrets.sh`** : Secret `postgres/openbao` (`basic-auth`), tiré au hasard, jamais recréé.
- **`deploy/bricks/openbao.sh`** : moteur `transit` et clé `config-secrets` ; politique `content-key-svc`
  (`update` sur `transit/encrypt/config-secrets` et `transit/decrypt/config-secrets`) ; auth `kubernetes` et
  son rôle ; moteur `database`, connexion `control_plane` sous `openbao` en `verify-full`, rôle
  `breakglass` (`CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}' IN
  ROLE breakglass`, 1 h ; révocation : `pg_terminate_backend` des sessions du rôle, puis `DROP ROLE IF EXISTS "{{name}}"`) ; `CREATE ROLE breakglass NOLOGIN` sous
  `SET ROLE openbao`. Amendé sur le VPS : la révocation par défaut du plugin (`REVOKE ALL` sur chaque
  schéma) échoue sur les objets de `migrator`, où `openbao` n'a aucun droit, et le rôle survivait à son
  bail ; il ne possède rien et ne tient ses droits que de `breakglass`.
- **`deploy/test-env/deploy.sh`** : `openbao.yaml` après les certificats ; attente du Job
  `helm-install-openbao` et du pod `openbao-0` `Running`.
- **`deploy/alerts/openbao.yaml`** et `openbao_test.yaml` : part sur 0, part sur absence, muette sur 1.
- **Garde** (`deploy/guard_test.go`, `checkBricks`) : le rôle géré `openbao` est `LOGIN CREATEROLE` ; le
  `HelmChart` `openbao` cible le namespace `openbao` en raft. Un cas faussé par règle.
- **Plan** : §010 coupé en 010a/010b, `breakglass` sous `openbao` ; la revue de la garde de déploiement
  (premier service) passe de 011a à 010b.
- **Preuve sur `contabo169`** : init, descellement, `openbao.sh` deux fois ; chiffrer et déchiffrer sous un
  jeton de la politique ; refus de `transit/keys/*` et d'une autre clé ; identifiants `breakglass` qui se
  connectent à `control_plane` puis expirent ; métrique `vault_core_unsealed` lue scellée et descellée ;
  entrée refusée depuis un pod non admis.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité, tests creux, code en trop) ; Fable, 2026-10-20. Aucun bloquant.

1. **Audit** : un device `file` sur stdout (06 §8, 07 §7.6 : émission auditée par OpenBao) ; un device
   non inscriptible bloque OpenBao, stdout l'est toujours ; valeurs HMACées. Amendé sur le VPS : déclaré
   dans la configuration d'`openbao.yaml`, gardé ; OpenBao 2 refuse de l'activer par l'API (« cannot
   enable audit device via API; use declarative, config-based audit device management instead »).
2. **Certificat du listener** : OpenBao ne relit `tls_cert_file` que sur SIGHUP, et redémarrer le pod le
   scelle. `duration: 8760h` ; plafond : le Secret se renouvelle vers J+240, et `CertificateExpiresSoon`
   ne voit jamais le certificat servi ; geste avant J+365 : supprimer le pod, desceller. Issue `debt`
   `go-live` (#122).
3. **Reconfiguration** : le chart met à jour en `OnDelete` ; un changement de `openbao.yaml` ne s'applique
   qu'au pod supprimé, puis descellé. Amendé au tour 2 : `deploy.sh` le détecte et échoue, jusqu'au geste
   humain. `configAnnotation` pose le checksum de la configuration sur le template, sans quoi un changement
   du seul HCL ne changerait pas la révision (gardé) ; `deploy.sh` compare la révision du pod `openbao-0` à
   `updateRevision`, après `observedGeneration`. Pas `currentRevision` : sous `OnDelete`, il ne suit pas
   le pod recréé (vu sur le VPS).
4. **Prometheus** (011a) : l'entrée 8200 d'OpenBao s'ouvre à lui, sinon `absent` sonne dès son arrivée.
5. **Here-string** : un `<<<` passe par un fichier temporaire sous bash < 5.1 ; `printf` dans le tube.
6. **Sessions `breakglass`** : `VALID UNTIL` n'agit qu'à la connexion ; une session ouverte survivrait au
   bail. `openbao` reçoit `pg_signal_backend` ; la révocation coupe les sessions du rôle puis le supprime,
   à la révocation comme à l'échéance. Coût (tour 2) : un détenteur du mot de passe d'`openbao`, qui ne vit
   que dans OpenBao et `postgres/openbao`, coupe des sessions non superutilisateur ; il ne lit ni n'écrit
   rien. Écartés : `createrole_self_grant = 'inherit'` ferait hériter `openbao` des droits de
   `breakglass` (des données contre de la disponibilité) ; `'set'` et un `SET ROLE "{{name}}"` dans la
   révocation seraient plus étroits, mais reposent sur la transaction du plugin, à reprendre si un audit
   exige le moindre privilège ici. Amendé sur le VPS : par `openbao.sh`, pas par `inRoles`. Posé,
   `inRoles` fait gérer à CloudNativePG toutes les adhésions du rôle : il a révoqué `breakglass` à
   `openbao` (`REVOKE "breakglass" FROM "openbao"`), qui en perdait l'`ADMIN OPTION`. La garde refuse
   `inRoles` sur `openbao`.
7. **Tests** : l'alerte prouve `for: 5m` (en attente et muette avant) et son retour au silence ; la garde
   exige `ha.enabled`, aucun mode `dev`, `storage "raft"` dans la configuration, un rôle `openbao` non
   `absent`, et nomme à part des valeurs illisibles.
8. **Retraits** : défauts du chart (`ui.enabled`, `authDelegator.enabled`), de Transit (`type`), du serveur
   (`ui`, `prometheus_retention_time`, coupé après lecture des métriques sur le VPS) ; `setNodeId` ; les
   noms `openbao` et `openbao.openbao` du certificat, que personne ne compose.

## Correctifs de revue, tours 2 et 3

Constats par tour : 15, 4, 2 ; aucun bloquant. Tour 2 arbitré par Fable (points 3 et 6 ci-dessus), tour 3
par l'humain (2026-10-20) : `deploy.sh` lit les deux révisions dans des variables avant de les comparer.
Placées dans le `[ … ]`, deux lectures en échec rendaient `[ "" = "" ]`, vrai, et le déploiement continuait ;
une seule en échec accusait une dérive. Prouvé par un faux `kubectl` : rouge, puis vert dans les quatre cas
(aucune dérive, dérive, une lecture en échec, deux).

## Preuve sur `contabo169`

Procédure jouée les 2026-10-19 et 20 depuis la branche (fichiers de `deploy/` copiés sur le VPS, timer de
déploiement suspendu pendant la preuve), sur le cluster de 007d-2b ; dernier passage au commit `0e17b63`,
OpenBao réinitialisé. Sorties expurgées des adresses et des secrets.

1. `namespaces.yaml`, `ca.yaml` : `Certificate` `openbao` `Ready`, un an, SAN `openbao.openbao.svc` et
   `127.0.0.1`. `secrets.sh` : Secret `postgres/openbao` créé, 64 caractères. `postgres.yaml` et
   `openbao.yaml` : Job `helm-install-openbao` `complete`, pod `openbao-0` `Running` et `0/1` (scellé),
   OpenBao v2.7.1, stockage raft, PVC de 1 Gi, aucun PDB ; rôle `openbao` `reconciled`.
2. Scellé : `/v1/sys/metrics` rend 200 et `vault_core_unsealed{cluster=""} 0`, sans
   `prometheus_retention_time`.
3. Init et descellement par l'humain (5 parts, seuil 3), hors dépôt : `Sealed false`, `HA Mode active`,
   `vault_core_unsealed 1`, pod `1/1`.
4. `openbao.sh` deux fois : le premier passage active `transit`, `kubernetes` et `database`, crée la clé
   (`aes256-gcm96`, ni exportable ni supprimable) et écrit politique, rôles et connexion (vérifiée par
   OpenBao à l'écriture, mot de passe lu sur l'entrée standard) ; le second saute activations et clé.
   Postgres : `breakglass` `NOLOGIN` ; `openbao` le détient avec l'`ADMIN OPTION`, et `pg_signal_backend`.
5. Pod de `sms` sous le SA `content-key-svc` (jeton projeté, audience `openbao`) : connexion, aller-retour
   `encrypt`/`decrypt` sur `config-secrets` identique (`vault:v1`) ; 403 sur `transit/keys/config-secrets`,
   `transit/encrypt/content`, `rotate`, `database/creds/breakglass`, `sys/policies/acl/content-key-svc` ;
   chiffré altéré : 400.
6. `database/creds/breakglass` sous le jeton racine : rôle `v-root-breakgla-…` qui se connecte à
   `control_plane`, membre de `breakglass`, `rolvaliduntil` à une heure, sans droit sur `goose_db_version`
   (#119). L'émission est au journal d'audit (`path`, `operation`, `display_name root`), comme les appels
   Transit du point 5.
7. Dérive (`configAnnotation` activé sans supprimer le pod) : `deploy.sh v0.4.0` échoue, « la
   configuration d'OpenBao n'est pas appliquée » ; pod supprimé, la vérification passe, pod scellé ;
   annotation injectée dans le template : échoue ; retirée : passe.
8. `bao lease revoke` pendant que la session du rôle est ouverte : la session tombe (« terminating
   connection due to administrator command »), le rôle disparaît, aucune erreur de révocation ; les
   adhésions d'`openbao` intactes, CloudNativePG sans `cannotReconcile`.

Rouges vus :

- **Jeton racine révoqué par la première version d'`openbao.sh`** : `generate-root` rend 403 « permission
  denied » (OpenBao ≥ 2.5.3) ; OpenBao réinitialisé (PVC supprimé), script amendé (arbitrage 8).
- **Bail `breakglass` irrévocable** (révocation par défaut du plugin) : à la révocation du jeton,
  « could not perform all revocation statements: ERROR: permission denied for sequence
  goose_db_version_id_seq », relancé sans fin, le rôle dynamique toujours dans Postgres ; script amendé.
- **`inRoles: [pg_signal_backend]`** sur `openbao` : CloudNativePG en `cannotReconcile`
  (« UPDATE_MEMBERSHIPS », `REVOKE "breakglass" FROM "openbao"` : « dependent privileges exist »), puis,
  le rôle dynamique orphelin supprimé, la révocation passe et `openbao` n'est plus membre de `breakglass` ;
  `inRoles` retiré, rôle `reconciled` et adhésions intactes.
- **Device d'audit par l'API** : « cannot enable audit device via API; use declarative, config-based audit
  device management instead » ; déclaré dans la configuration.
- **Dérive lue sur `currentRevision`** : après recréation du pod, `currentRevision` reste l'ancienne
  révision (`updatedReplicas 1`) ; comparaison faite sur la révision du pod.
- **Entrée d'OpenBao** depuis un pod de `default` (sans politique de sortie) : `connection refused` sur 8200.
