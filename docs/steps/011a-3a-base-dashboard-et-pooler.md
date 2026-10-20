# Step 011a-3a — Base `dashboard` sur le cluster, `migrate` et `catalog-apply` multi-base, Pooler

Plan : `docs/plan/composants.md` §011, première des deux PR de 011a-3 (humain, 2026-10-20). Arbitrages antérieurs :
`docs/steps/011a-1-prometheus-et-alertmanager.md` (1 à 22), `docs/steps/011a-2a-schema-par-base.md` (23 à 36),
`docs/steps/011a-2b-dashboard-coeur.md` (37 à 44). Spec : `docs/spec/02-architecture.md` §2.3. Règles :
`.claude/rules/ci-deploy.md`, `.claude/rules/migrations.md`, `.claude/rules/tests.md`. Mode : PR.

Ce document porte les arbitrages de **toute la 011a-3** (45 à 58) ; 011a-3b y renvoie.

## Contexte

- 011a-2a a livré `migrations/dashboard` et `migrations/grants/dashboard.sql` ; `cmd/migrate` n'applique que
  `control_plane`, et `cmd/catalog-apply` que `control_plane.sql`. Le rôle `dashboard` existe sur le cluster
  (release v0.10.0 et suivantes), mais pas la base.
- Le cluster CloudNativePG n'a qu'une base, née d'`initdb` ; aucun pool de connexions.
- Dettes du territoire : aucune pour 011a-3a.

**Terminé quand** : sur l'environnement de test, la base `dashboard` existe, migrée par `migrate` ; `catalog-apply` y
applique ses droits ; le rôle `dashboard` s'y connecte à travers le Pooler et lit ses tables, mais pas celles de
`control_plane`.

## Arbitrages

Fable, 2026-10-20, sauf I à L, tranchés par l'humain le même jour sur recommandation.

45. **Coupe** (humain, I) : 011a-3a, base sur le cluster, `migrate` et `catalog-apply` multi-base, Pooler, sans `cmd/`
    neuf ; 011a-3b, `cmd/dashboard`, HTTP, Mailpit, Secret, manifest, `deploy.sh`, preuve `/readyz`.
46. **Surface HTTP** (3b) : les sept opérations qui ont un cœur, et elles seules, par `include-operation-ids`
    d'oapi-codegen. Une opération non servie rend le 404 `not_found` du contrat. `listAccessLog` exige `audit:read`,
    que personne ne détient avant 012 : 403 `forbidden_scope`. La règle « MFA obligatoire » ne s'applique pas en
    011a-3 (aucune permission au-delà des lectures, aucune élévation) ; 011b l'ajoute avec `Elevated`.
47. **Pile** (3b) : `internal/dashboard/http/oapi-codegen.yaml` (`std-http-server`, `strict-server`, `models`,
    `embedded-spec`, `include-operation-ids`), généré `bff.gen.go` sous `go generate`. Dépendances directes :
    `kin-openapi v0.149.0`, `oapi-codegen/runtime v1.7.0`, `nethttp-middleware v1.2.0` (validation, `AuthenticationFunc`,
    `ErrorHandler`). Origin, 413 (16 KiB) et 415 en middlewares avant la validation.
48. **HTTP dans le pod, cookie `Secure`** (humain, J) : `http://localhost` par `port-forward` ; Chrome et Firefox
    l'admettent, Safari non (limite assumée jusqu'au répartiteur, 026).
49. **Configuration** (3b) : `HTTP_ADDR`, `DASHBOARD_PUBLIC_URL` (URL absolue sans chemin ; l'origine attendue en
    dérive), `DASHBOARD_BOOTSTRAP_EMAIL` (exigé pour toujours, humain, L), `SMTP_ADDR`, `SMTP_FROM`, `SMTP_USERNAME`,
    `SMTP_PASSWORD_FILE` (exigé si un utilisateur est donné), `TRUSTED_PROXIES`, `COOKIE_KEY_FILE`. `Load` juge la
    forme, `run` refuse l'incohérence. `TLS_CA_FILE` vérifie Mailpit.
50. **Boucles** (3b) : livraison toutes les 5 s jusqu'à épuisement ; purge toutes les minutes ; bootstrap avant de
    servir, bloquant. `/readyz` sans dépendance à Postgres (humain, K). `pool.Close()` après le superviseur (#12).
51. **Métrique** (3b) : jauge `dashboard_access_links_undeliverable` et alerte `AccessLinkUndeliverable`
    (`deploy/alerts/dashboard.yaml`).
52. **Base sur le cluster** : `Database` de CloudNativePG (`owner: migrator`, `databaseReclaimPolicy: retain`) ;
    `deploy.sh` attend `status.applied` avant les Jobs. `migrate` et `catalog-apply` dérivent l'URL de chaque base
    du chemin de `POSTGRES_URL`, une base après l'autre ; la garde exige un `Database` pour chaque base hors celle
    d'`initdb`.
53. **Pooler** `postgres-pooler-rw` : `type: rw`, deux instances, `poolMode: transaction`, `default_pool_size` 10 ;
    authentification intégrée de CloudNativePG ; les SAN du Pooler s'ajoutent au certificat du serveur, que le Pooler
    réutilise ; ses pods portent `cnpg.io/cluster`, que la NetworkPolicy de Postgres et la sortie générée par
    `internal/netpol` couvrent déjà. pgx garde son mode par défaut (PgBouncer gère les instructions préparées) ;
    repli `default_query_exec_mode=exec` si la preuve rougit. Les Jobs restent en direct.
54. **Mailpit** (3b) : brique `deploy/bricks/mailpit.yaml`, `axllent/mailpit:v1.31.4`, STARTTLS exigé, certificat de
    la CA interne ; entrée 1025 aux seuls pods `dashboard` ; `SMTP_ADDR` en `<nom>.<ns>.svc:<port>` ouvre la sortie.
55. **Secret `dashboard`** (3b) : `postgres-password`, `cookie-key` et `postgres-url` (par le Pooler) ;
    `secrets.sh` ajoute une clé absente à un Secret existant.
56. **Manifest** (3b) : deux répliques, PDB `minAvailable: 1`, sondes sur ops, grâce de 125 s. Derrière
    `port-forward`, toutes les connexions viennent de 127.0.0.1 : cinq échecs verrouillent la source pour tous. `/`
    n'est pas monté avant 011a-4.
57. **Dettes** (3b) : #12 soldée ; #9 moitié sondes soldée, moitié CSP reportée (026) ; #20 en 011a-4 ; #43 une ligne,
    reste ouverte.
58. **Tranché d'avance** (3b) : Origin et 415 sur toute méthode non sûre ; 422 `validation_error` avec `errors[]` ;
    cookie sans `Max-Age`, que `logout` périme ; `X-Forwarded-For` lu seulement depuis `TRUSTED_PROXIES` ; `Me` :
    `permissions []`, `elevated false`, facteurs à zéro, `idleTimeoutSeconds 900` ; aucun journal de requête ;
    `ReadHeaderTimeout` 5 s, `IdleTimeout` 60 s, sans h2.

## Design arrêté

- **`internal/platform/pgdial`** : `Database(url, name)` rend l'URL au chemin remplacé, refuse une URL illisible.
- **`cmd/migrate`** : pour chaque base de `migrations.Databases`, un provider goose sous son verrou de session ;
  `control_plane` d'abord.
- **`cmd/catalog-apply`** : pour chaque base, `postgres.Apply(Grants(base))` connecté à cette base, le même
  répertoire de secrets.
- **`deploy/bricks/postgres.yaml`** : `Database` `dashboard`, `Pooler` `postgres-pooler-rw`.
- **`deploy/bricks/ca.yaml`** : les trois SAN du Pooler sur `postgres-server`.
- **`deploy/test-env/deploy.sh`** : attente de `database/dashboard` appliquée et du rollout du Pooler, avant les Jobs.
- **`deploy/guard_test.go`** : un `Database` par base hors `initdb`, au propriétaire `migrator`, en `retain` ; le Pooler
  au cluster `postgres`, en mode transaction.
- **Tests** : `migrate` applique les deux bases et se rejoue à vide ; `catalog-apply` applique les deux fichiers et le
  rôle `dashboard` lit ses tables ; `pgdial.Database` ; cas faussés de la garde.
- **Preuve sur `contabo169`** : `Database` appliquée ; Jobs `migrate` et `catalog-apply` de la branche réussis ; un pod
  au label `catalog-apply` se connecte en `dashboard` par `postgres-pooler-rw`, lit `dashboard.operators`, est refusé
  sur `control_plane.audit_log`.

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.11.0. Le timer de déploiement est suspendu pendant la
preuve, puis réarmé. Les images de `migrate` et `catalog-apply` sont construites localement et importées dans k3s sous
le tag `proof`.

1. `ca.yaml` : `postgres-server` réémis avec les SAN du Pooler. `postgres.yaml` : `database/dashboard` à
   `applied=true`, `postgres-pooler-rw` actif, ses deux répliques prêtes.
2. `migrate` : `control_plane` à jour, `dashboard` migrée (`"database":"dashboard","version":1`), après les rejets
   d'admission de kube-router, que `pgdial.Retry` absorbe.
3. `catalog-apply` : secrets posés pour `admin_api_svc` et `dashboard` dans chaque base.
4. Un pod au label `catalog-apply` se connecte en `dashboard` par `postgres-pooler-rw` en `verify-full` sous la CA
   interne : il lit `dashboard.operators` et se voit refuser le schéma `control_plane` (« permission denied for schema
   control_plane »).

Rouge vu : un premier pod sans le montage de la CA a échoué au TLS, comme il le devait.
