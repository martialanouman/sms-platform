# Step 007b-2 — Livraison : `catalog-apply`, GRANT en production

Plan : `docs/plan/socle.md` §007, troisième des six PR. Spec : `docs/spec/02-architecture.md` §2.2 (Jobs),
§2.5 (une identité par service sur chaque magasin), §2.6 (portabilité) ; `docs/spec/10-exploitation.md`
§10.2. Décision : `docs/design/06-confiance-interne.md` §5.1. Reports de `docs/steps/006b-grants.md`. Règles :
`.claude/rules/migrations.md`, `.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

006b génère `migrations/grants/control_plane.sql` depuis `contracts/grants/*.yaml` : une transaction qui
crée chaque rôle de service `NOLOGIN` s'il manque (en refusant d'adopter un rôle qui n'est pas simple), lui
retire tout puis lui accorde exactement sa déclaration. Seul le test l'applique. Une seule déclaration
existe : `admin-api-svc`. `catalog-apply` (007b-1) applique le catalogue Kafka et ne touche pas Postgres.

Reports de 006b, portés au plan : `catalog-apply` applique le fichier sous `migrator`, une seule fois à la
fois, en échouant sur la première erreur ; il pose `LOGIN` et le secret de chaque rôle de service ; il
retire les droits d'un rôle sorti des déclarations, que le fichier ne nomme plus.

La spec ne dit pas comment un service s'authentifie auprès de Postgres. 02 §2.6 : « une brique qui
n'existe pas à l'identique des deux côtés n'entre pas » ; RDS et Cloud SQL n'authentifient pas un rôle par
certificat client : le secret est un mot de passe.

**Terminé quand** (part de 007b-2) : sur un Postgres réel migré, `catalog-apply` laisse à chaque rôle
déclaré exactement ses droits et le `LOGIN` avec son secret, et à un rôle retiré des déclarations ni droit
ni `LOGIN` ; deux exécutions simultanées réussissent ; une seconde exécution ne change rien.

## Arbitrages

Fable a tranché le 2026-10-19 (P1, P4, P5 tenues ; P2, P6, P7, P8 amendées ; P3 rejetée) :

- **P1 — Authentification** : mot de passe SCRAM-SHA-256, portable (02 §2.6). `catalog-apply` envoie le
  vérificateur calculé côté client (`SCRAM-SHA-256$4096:<sel>$<StoredKey>:<ServerKey>`, 4096 itérations,
  sel de 16 octets), jamais le clair : `log_min_error_statement = error` est le défaut de Postgres, un
  `ALTER ROLE … PASSWORD` qui échoue écrirait sinon le secret dans un journal que l'exploitant ne contrôle
  pas sur RDS ou Cloud SQL. Pas de SASLprep : un secret hors ASCII imprimable est refusé.
- **P2 — Source du secret** (amendée) : 06 §6 borne le contenu du Secret d'un service, pas qui le monte ;
  un Secret qui porterait tous les mots de passe serait `gateway-secrets`, supprimé ; écrire les Secrets
  depuis le Job mettrait client-go, un compte de service et du RBAC. `catalog-apply` monte un volume
  `projected`, une source par service déclaré (clé `postgres-password`, fichier `<rôle>`), sous
  `POSTGRES_ROLE_SECRETS_DIR` (`config`, sans défaut). Couplage neuf : ajouter `contracts/grants/<svc>.yaml`
  ajoute une source à `deploy/k8s/catalog-apply.yaml` ; la garde de livraison le tient,
  `.claude/rules/contracts.md` le nomme.
- **P3 — Secret absent** (rejetée) : **refus**. Une source `projected` absente bloque déjà le pod avant Go ;
  007d-1 crée le Secret de chaque service déclaré avant tout service (séquencement porté au plan), rien
  n'est différé.
- **P4 — Une seule fois à la fois** : `pg_advisory_xact_lock` après le `BEGIN` du fichier généré ; ses deux
  appelants le prennent, le `CREATE ROLE` conditionnel ne court plus.
- **P5 — Première erreur** : le fichier s'exécute tel quel, une transaction, protocole simple ; une erreur
  l'annule entier et aucun mot de passe n'est touché.
- **P6 — Rôle retiré** (amendée) : la marque d'un rôle géré est l'`ADMIN OPTION` de `migrator` sur lui, que
  Postgres 16+ lui donne sur chaque rôle qu'il crée et sans laquelle il ne peut ni l'altérer ni le
  commenter (vérifié sur Postgres 18). Rôles retirés = gérés moins nommés : le fichier leur révoque tables,
  schéma et base de `control_plane` et pose `NOLOGIN`. Ni `COMMENT ON ROLE`, ni motif de nom, ni rôle de
  groupe ; pas de `DROP ROLE` (réversible ; un objet possédé ailleurs ferait échouer le fichier).
- **P7 — Câblage** (amendée) : un Job pour Kafka et Postgres (02 §2.2), Postgres après Kafka. Le fichier
  pose `LOGIN` à chaque rôle déclaré (sans mot de passe, SCRAM refuse la connexion : fenêtre sûre) ; `Go`
  liste ensuite les rôles gérés qui ont `LOGIN` et leur pose leur vérificateur depuis `<dir>/<rôle>` : une
  seule source de vérité, le fichier commité. `run` refuse tout manque (brokers, mTLS, `POSTGRES_URL`,
  `POSTGRES_ROLE_SECRETS_DIR`) avant toute connexion. Le journal ne porte ni URL, ni secret, ni vérificateur.
- **P8 — Tests** (amendée) : voir le design. La preuve « le serveur ne reçoit pas le clair » est un journal
  serveur sous `log_statement = 'all'` qui porte l'`ALTER ROLE` et pas le secret : la présence de
  l'instruction prouve que le journal capture.

Amendé ici : comme `ALTER ROLE` exige l'`ADMIN OPTION`, un rôle existant n'est adopté que si `migrator` la
détient ; le test d'adoption de 006b, qui crée le rôle sous `postgres`, la lui accorde (`INHERIT FALSE,
SET FALSE`, que la garde d'adoption admet) comme le ferait un superutilisateur qui lui confie un rôle.

## Design arrêté

- `internal/schema` :
  - `GrantsSQL` : `pg_advisory_xact_lock` après `BEGIN` ; `ALTER ROLE … LOGIN` par rôle déclaré ; un bloc
    final qui, pour chaque rôle dont `migrator` a l'`ADMIN OPTION` et que le fichier ne nomme pas, révoque
    ses droits sur les tables, le schéma et la base `control_plane` et pose `NOLOGIN` ;
  - `SCRAMVerifier(password string, salt []byte) (string, error)` : refuse un secret vide ou hors ASCII
    imprimable.
- `migrations/grants/control_plane.sql` régénéré ; `migrations.Grants() []byte` l'embarque.
- `internal/schema/postgres` : `Apply(ctx, *pgx.Conn, sql []byte, secrets fs.FS) error` exécute le
  fichier, liste les rôles gérés qui ont `LOGIN`, lit chaque secret, pose le vérificateur (sel tiré par
  `crypto/rand`) ; rend la liste des rôles traités.
- `internal/platform/config` : `POSTGRES_ROLE_SECRETS_DIR` → `Infra.PostgresRoleSecretsDir`, sans défaut.
- `cmd/catalog-apply` : `run` refuse brokers vides, `POSTGRES_URL` vide, `POSTGRES_ROLE_SECRETS_DIR` vide,
  mTLS absent, avant toute connexion ; Kafka puis Postgres sous `migrator` ; journalise les rôles dont le
  secret est posé, sans secret.
- `deploy/k8s/catalog-apply.yaml` : `POSTGRES_URL` par le Secret `catalog-apply` ; volume `projected` des
  secrets de rôle. `deploy/guard_test.go` : les sources de ce volume sont exactement les déclarations de
  `contracts/grants/`, chacune `<svc>` / `postgres-password` / `<rôle>`.
- `.claude/rules/contracts.md` : le couplage `grants/` → volume de `catalog-apply`. Plan §007d-1 : le Secret
  `postgres-password` de chaque service déclaré et le Secret `catalog-apply`.
- `internal/platform/pgtest` : `DB.Logs(t)` rend le journal du serveur.
- Tests :
  - unitaire : vérificateur conforme au RFC 7677 sur un vecteur connu, refus d'un secret vide ou non ASCII ;
  - intégration (`internal/schema/postgres`, Postgres réel) : droits exacts et `LOGIN` du rôle déclaré,
    connexion avec son secret et refus avec un autre ; un rôle géré retiré n'a plus ni droit ni `LOGIN` ;
    deux `Apply` simultanés derrière une barrière, sur une base sans le rôle, réussissent tous les deux ;
    une seconde passe ne change pas les droits ; un secret absent fait échouer `Apply` sans toucher au
    mot de passe ; sous `log_statement = 'all'`, le journal porte l'`ALTER ROLE` et pas le secret ;
  - `migrations` : les tests de 006b passent sur le fichier régénéré ;
  - `run` refuse chaque manque sans connexion ; la garde de livraison tombe sur une source manquante ou en
    trop.
