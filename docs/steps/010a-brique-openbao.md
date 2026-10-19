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
   racine et le mot de passe d'`openbao` passent par l'entrée standard, jamais par un argument ; le jeton
   est révoqué en fin de script. `deploy.sh` attend le pod `Running`, pas le descellement : un timer
   bloqué sur un OpenBao scellé retiendrait toutes les releases.
9. **Alerte** `OpenBaoSealed` : `vault_core_unsealed == 0`, et `absent` parce qu'un OpenBao scellé rend 503
   sur ses métriques. Elle entre avec la brique qui émet la métrique ; le nom se vérifie sur le VPS.
10. **Dettes** : droits de `breakglass` (014a) ; authentification humaine à OpenBao pour lire
    `database/creds/breakglass` (`go-live`) ; snapshot chiffré d'OpenBao (10 §10.3, `go-live`). #108 :
    reportée, hors territoire.

## Design arrêté

- **`deploy/bricks/namespaces.yaml`** : namespace `openbao`.
- **`deploy/bricks/openbao.yaml`** : le `HelmChart` `openbao` (repo `https://openbao.github.io/openbao-helm`,
  chart 0.30.2, cible `openbao`) et la `NetworkPolicy` d'entrée d'OpenBao : 8200 depuis les pods
  `app.kubernetes.io/name: content-key-svc` de `sms`.
- **`deploy/bricks/ca.yaml`** : `Certificate` `openbao` (namespace `openbao`, `server auth`, `openbao`,
  `openbao.openbao`, `openbao.openbao.svc`) ; son `ca.crt` vérifie aussi Postgres.
- **`deploy/bricks/postgres.yaml`** : rôle géré `openbao` (`LOGIN CREATEROLE`, Secret `openbao`) ; entrée
  5432 depuis les pods OpenBao.
- **`deploy/bricks/secrets.sh`** : Secret `postgres/openbao` (`basic-auth`), tiré au hasard, jamais recréé.
- **`deploy/bricks/openbao.sh`** : moteur `transit` et clé `config-secrets` ; politique `content-key-svc`
  (`update` sur `transit/encrypt/config-secrets` et `transit/decrypt/config-secrets`) ; auth `kubernetes` et
  son rôle ; moteur `database`, connexion `control_plane` sous `openbao` en `verify-full`, rôle
  `breakglass` (`CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}' IN
  ROLE breakglass`, 1 h) ; `CREATE ROLE breakglass NOLOGIN` sous `SET ROLE openbao` ; révocation du jeton.
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
