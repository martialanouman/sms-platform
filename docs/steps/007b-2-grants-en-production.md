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
  appelants le prennent, le `CREATE ROLE` conditionnel ne court plus. Amendé en TDD : les `ALTER ROLE …
  PASSWORD`, hors de la transaction du fichier, couraient encore (`tuple concurrently updated`) ; `Apply`
  prend `pg_advisory_lock` en session autour de tout, et le verrou du fichier, devenu sans appelant
  concurrent, est retiré.
- **P5 — Première erreur** : le fichier s'exécute tel quel, une transaction, protocole simple ; une erreur
  l'annule entier et aucun mot de passe n'est touché.
- **P6 — Rôle retiré** (amendée) : la marque d'un rôle géré est l'`ADMIN OPTION` de `migrator` sur lui, que
  Postgres 16+ lui donne sur chaque rôle qu'il crée et sans laquelle il ne peut ni l'altérer ni le
  commenter (vérifié sur Postgres 18). Rôles retirés = gérés moins nommés : le fichier leur révoque tables,
  schéma et base de `control_plane` et pose `NOLOGIN`. Ni `COMMENT ON ROLE`, ni motif de nom, ni rôle de
  groupe ; pas de `DROP ROLE` (réversible ; un objet possédé ailleurs ferait échouer le fichier).
- **P7 — Câblage** (amendée) : un Job pour Kafka et Postgres (02 §2.2), l'ordre indifférent ; Postgres
  d'abord, pour qu'un test de `run` construise le câblage Postgres réel avant que Kafka échoue faute de broker. Le fichier
  pose `LOGIN` à chaque rôle déclaré (sans mot de passe, SCRAM refuse la connexion : fenêtre sûre) ; `Go`
  liste ensuite les rôles gérés qui ont `LOGIN` et leur pose leur vérificateur depuis `<dir>/<rôle>` : une
  seule source de vérité, le fichier commité. `run` refuse tout manque (brokers, mTLS, `POSTGRES_URL`,
  `POSTGRES_ROLE_SECRETS_DIR`) avant toute connexion. Le journal ne porte ni URL, ni secret, ni vérificateur.
- **P8 — Tests** (amendée) : voir le design. La preuve « le serveur ne reçoit pas le clair » est un journal
  serveur sous `log_statement = 'all'` qui porte l'`ALTER ROLE` et pas le secret : la présence de
  l'instruction prouve que le journal capture.

Second arbitrage de Fable (2026-10-19), sur la revue :

- Un rôle retiré perd aussi son mot de passe (`NOLOGIN PASSWORD NULL`) : re-déclaré, il recevrait sinon
  `LOGIN` avec un ancien secret, et y resterait si `Apply` échouait après le fichier. Un rôle adopté qui a
  déjà un mot de passe le garde jusqu'au vérificateur : résiduel accepté, l'adoption suit un geste
  délibéré de superutilisateur ; effacer le mot de passe à chaque passage couperait les reconnexions des
  services en marche.
- La marque « `ADMIN OPTION` de `migrator` » vaut parce que `migrator` ne crée aucun rôle hors du fichier
  généré, migrations comprises ; `breakglass` et le rôle d'OpenBao naissent sous un autre rôle (06 §8,
  plan §007d-1). Le fichier refuse de s'appliquer sous un autre rôle que `migrator` : sous le maître d'un
  RDS, il retirerait tout ce que ce maître a créé.
- Un secret fait 32 caractères au moins : sous `log_statement = 'ddl'`, le vérificateur est journalisé, et
  un secret court se casse hors ligne.
- Les droits de `PUBLIC` sur la base (`CONNECT`, `TEMP`) restent : `CONNECT` est celui qu'une déclaration
  implique, `TEMP` ne touche aucune donnée ; la migration qui créera une deuxième base révoquera `CONNECT`
  de `PUBLIC` (plan §006).
- Rejeté : un `ROLLBACK` avant de relâcher le verrou ; l'appelant ferme la connexion sur toute erreur.

Amendé ici : comme `ALTER ROLE` exige l'`ADMIN OPTION`, un rôle existant n'est adopté que si `migrator` la
détient ; le test d'adoption de 006b, qui crée le rôle sous `postgres`, la lui accorde (`INHERIT FALSE,
SET FALSE`, que la garde d'adoption admet) comme le ferait un superutilisateur qui lui confie un rôle ; son
cas « membre d'un autre rôle » passe par `pg_read_all_data`, `GRANT migrator TO admin_api_svc` devenant
circulaire.

## Design arrêté

- `internal/schema` :
  - `GrantsSQL` : `ALTER ROLE … LOGIN` par rôle déclaré ; un bloc
    final qui, pour chaque rôle dont `migrator` a l'`ADMIN OPTION` et que le fichier ne nomme pas, révoque
    ses droits sur les tables, le schéma et la base `control_plane` et pose `NOLOGIN PASSWORD NULL` ; en
    tête, un refus si `current_user` n'est pas `migrator` ;
  - `SCRAMVerifier(password string, salt []byte) (string, error)` : refuse un secret de moins de 32
    caractères ou hors ASCII imprimable.
- `migrations/grants/control_plane.sql` régénéré ; `migrations.Grants() []byte` l'embarque.
- `internal/schema/postgres` : `Apply(ctx, *pgx.Conn, sql []byte, secrets fs.FS) ([]string, error)` exécute le
  fichier, liste les rôles gérés qui ont `LOGIN`, lit tous les secrets puis pose les vérificateurs (sel tiré
  par `crypto/rand`), sous `pg_advisory_lock` en session ; rend la liste des rôles traités.
- `internal/platform/config` : `POSTGRES_ROLE_SECRETS_DIR` → `Infra.PostgresRoleSecretsDir`, sans défaut.
- `cmd/catalog-apply` : `run` refuse brokers vides, `POSTGRES_URL` vide, `POSTGRES_ROLE_SECRETS_DIR` vide,
  mTLS absent, avant toute connexion ; Postgres sous `migrator` puis Kafka ; journalise les rôles dont le
  secret est posé, sans secret.
- `deploy/k8s/catalog-apply.yaml` : `POSTGRES_URL` par le Secret `catalog-apply` ; volume `projected` des
  secrets de rôle. `deploy/guard_test.go` : les sources de ce volume sont exactement les déclarations de
  `contracts/grants/`, chacune `<svc>` / `postgres-password` / `<rôle>`.
- `.claude/rules/contracts.md` : le couplage `grants/` → volume de `catalog-apply`. Plan §007d-1 : le Secret
  `postgres-password` de chaque service déclaré et le Secret `catalog-apply`.
- `internal/platform/pgtest` : `DB.Logs(t)` rend le journal du serveur ; `DB.AsWith(role, password)`.
- Tests :
  - unitaire : forme du vérificateur, refus d'un secret vide ou hors ASCII imprimable ; sa justesse a pour
    oracle Postgres lui-même (une connexion avec le secret passe, une autre rend 28P01) ;
  - intégration (`internal/schema/postgres`, Postgres réel) : droits exacts et `LOGIN` du rôle déclaré,
    connexion avec son secret et refus avec un autre ; un rôle géré retiré n'a plus ni droit ni `LOGIN` ;
    deux `Apply` simultanés derrière une barrière, sur une base sans le rôle, réussissent tous les deux ;
    une seconde passe ne change pas les droits ; un secret absent fait échouer `Apply` sans toucher au
    mot de passe ; sous `log_statement = 'all'`, le journal porte l'`ALTER ROLE` et pas le secret ;
  - `migrations` : les tests de 006b passent sur le fichier régénéré ;
  - `run` refuse chaque manque sans connexion ; sur un Postgres réel, il pose le secret (le rôle se connecte)
    et journalise le rôle sans secret ni URL, puis parle TLS à Kafka ; un secret absent le fait échouer ; la
    garde de livraison tombe sur une source manquante, en trop, sous une autre clé ou un autre chemin.
- `.golangci.yml` : G101 exclu du test de la configuration, qui donne un chemin à
  `POSTGRES_ROLE_SECRETS_DIR`.

## Revue

**Tour 1** (trois axes, aucun bloquant ; deux majeurs de justesse au second arbitrage de Fable, ci-dessus) :

- Retenus : un rôle retiré gardait son mot de passe ; la marque `ADMIN OPTION` visait tout rôle créé par
  `migrator`, et le fichier s'appliquait sous n'importe quel rôle ; aucune entropie exigée du secret ;
  sel de 80 bits (`rand.Text`) au lieu de 16 octets ; `Apply` sans test pour un rôle retiré ou étranger
  (un filtre de sa requête survivait), pour l'ordre « lire tous les secrets, puis poser » (une seule
  déclaration ne le montrait pas), pour le sel tiré ; DEL admis sans test ; le test du journal supposait le
  rechargement de configuration et le vidage des journaux faits ; signature d'`Apply` fausse au design ;
  helpers de connexion recopiés (`pgtest.DB.AsWith`).
- Écartés : la garde ne vérifie pas le montage du volume des secrets (un montage faux échoue bruyamment au
  premier passage, et le design ne l'exige pas) ; `roleSecrets` relit le manifest que `checkManifest` a
  décodé (deux lignes, un seul document) ; `ROLLBACK` avant de relâcher le verrou (arbitrage) ; aucune borne
  de temps sur le verrou (Job borné en 007d-2, plan).
