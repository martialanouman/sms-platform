# Step 011a-2a — Schéma et GRANT par base, base `dashboard`

Plan : `docs/plan/composants.md` §011, première des deux PR de 011a-2 (humain, 2026-10-20, sur recommandation de
Fable). Arbitrages de toute la 011a : `docs/steps/011a-1-prometheus-et-alertmanager.md` (1 à 22). Spec :
`docs/spec/08-dashboard.md` §8.3, §8.9 ; `docs/spec/02-architecture.md` §2.3. Décisions : `docs/design/05-autorisation-et-audit.md`
§4.3, §4.4 ; `docs/design/06-confiance-interne.md` §5.1 ; `docs/design/09-conventions-du-socle.md` C3. Précédents :
`docs/steps/006a-audit.md`, `docs/steps/006b-grants.md`, `docs/steps/007b-2-grants-en-production.md`. Règles :
`.claude/rules/migrations.md`, `.claude/rules/contracts.md`, `.claude/rules/tests.md`. Mode : PR.

Ce document porte les arbitrages de **toute la 011a-2** (23 à 36) ; 011a-2b y renvoie.

## Contexte

- `internal/schema` et `tools/grants` ne connaissent qu'une base : `ParseGrants` refuse toute clé autre que
  `control_plane:`, `GrantsSQL` écrit en dur le nom de la base et du schéma, `tools/grants` n'écrit que
  `migrations/grants/control_plane.sql`.
- La boucle de retrait de `GrantsSQL` passe `NOLOGIN PASSWORD NULL` tout rôle que `migrator` administre et que le
  fichier ne déclare pas : un rôle déclaré dans une autre base y serait désactivé.
- `pgtest` ne crée que `control_plane` ; `migrations` n'embarque que `control_plane`.
- 007b-2 a laissé `CONNECT` à `PUBLIC` sur `control_plane` et promis que la migration qui créerait une deuxième base
  le révoquerait. L'identité `breakglass` d'OpenBao (010a) se connecte à `control_plane` par ce droit.
- Dettes du territoire : #7 (rétention du journal d'accès), soldée ici par la porte et le plancher.

**Terminé quand** : la base `dashboard` migre en montée, descente et remontée sous `migrator` ; chaque table y est
classée ; le rôle `dashboard` n'y détient que sa déclaration et n'atteint aucune table de `control_plane`, et
inversement ; le journal d'accès n'admet que l'ajout, sauf la porte de purge au-delà de 8 760 h.

## Arbitrages

Fable, 2026-10-20, sauf E à H, tranchés par l'humain le même jour sur recommandation.

23. **Coupe** (humain, E) : 011a-2a, schéma et GRANT par base, base `dashboard`, aucun code du dashboard ; 011a-2b, cœur
    `internal/dashboard`, adaptateurs Postgres et SMTP.
24. **sqlc** (humain, F), en 011a-2b : `sql_package: pgx/v5`, `emit_methods_with_db_argument`, `sqlc.yaml` à la racine,
    requêtes sous `internal/dashboard/postgres/queries/`, schéma lu dans `migrations/dashboard`, tables qualifiées,
    génération sous `go generate`. Coût : une exclusion `revive` nommée sur le généré (C3, `migrations.md`).
25. **argon2id dans le cœur** (011a-2b) : `golang.org/x/crypto/argon2` entre dans la liste blanche de depguard du cœur
    (ADR 0025 : une bibliothèque pure s'y ajoute quand un cœur en a besoin) ; créneaux, PHC `m=65536,t=3,p=4` et
    hachage factice dans `internal/dashboard`.
26. **Horloge** (011a-2b) : la comparaison au temps vit dans le SQL (`now()` de Postgres), les durées sont des
    constantes du cœur passées au port ; le cœur ne lit jamais `time.Now()`. Une mutation de constante tombe au test du
    cœur, une mutation de prédicat au test de l'adaptateur, qui antidate les lignes.
27. **GRANT par base** : `contracts/grants/<service>.yaml` à clés par base (`control_plane:`, `dashboard:`), une base
    inconnue refusée ; chaque base a son classement (`migrations/<base>/audit.yaml`) et son fichier généré
    (`migrations/grants/<base>.sql`), au nom de schéma égal à celui de la base. Chaque fichier : tout rôle de
    **l'union** des déclarations naît ou est adopté et reçoit `LOGIN` (l'ordre d'application des bases est libre) ;
    les droits de **cette** base sont remis à zéro puis accordés selon la déclaration, `CONNECT` compris (accordé si le
    rôle y déclare une table, retiré sinon) ; le retrait (`NOLOGIN PASSWORD NULL`) ne vise que les rôles absents de
    l'union. `Check` ne change pas : dans une base sans `audit_capture()`, une table `diff` tombe déjà en « diff
    without audit_capture ».
28. **Frontière** : `cmd/migrate` et `cmd/catalog-apply` multi-base, avec la base sur le cluster, en 011a-3 ; 011a-2a
    se prouve par testcontainers. Entrent ici, parce que la garde de livraison l'exige dès qu'une déclaration existe :
    `contracts/grants/dashboard.yaml`, les fichiers de GRANT régénérés et la source `dashboard` du volume
    `role-secrets` de `catalog-apply`. Le rôle `dashboard` naît donc sur le cluster à la release suivante, par
    `control_plane.sql` (union), avec le SCRAM du Secret que `secrets.sh` crée déjà.
29. **Tables** de `migrations/dashboard/00001_dashboard.sql`, toutes `exempt` :
    - `operators` : `id uuid` (uuidv7), `email` unique sur `lower(email)` (l'unicité sans verrou de l'arbitrage 3),
      `display_name` (l'e-mail pour l'opérateur né par `DASHBOARD_BOOTSTRAP_EMAIL`, éditable en 012),
      `password_hash` nullable (né sans mot de passe), `status` `active` ou `disabled`, `created_at`.
    - `access_links`, l'outbox : une ligne par opérateur, `kind` `activation` ou `reset`, `token_hash` unique
      (tiré à l'envoi), `expires_at`, `sent_at`, `attempts`, `next_attempt_at`.
    - `sessions` : `token_hash` unique, `created_at`, `expires_at` (12 h), `last_seen_at`, `challenge_hash` unique et
      `challenge_expires_at` (défi de 5 min, arbitrage 6). Ni `elevated_at` (011b) ni `noc` (012).
    - `login_attempt_counters` : `subject bytea` (HMAC sous domaine, arbitrage 33), `failures`, `last_failure_at`.
    - `access_log` : `id`, `at`, `actor` et `target` nullables sans clé étrangère (le journal tient ses auteurs),
      `event` et `outcome` sous `CHECK` en miroir de l'enum du contrat, clé `(at, id)`.
30. **Rétention** (humain, G) : la porte de 006a reprise telle quelle sur `access_log` (`UPDATE` et `TRUNCATE`
    refusés en 42501 ; `DELETE` admis sous `SET LOCAL audit_log.purge = 'on'` et au-delà de `8760 hours`), même nom de
    réglage pour un seul geste d'exploitation ; `delete` accordé à personne, la purge est un geste de `migrator`. #7
    soldée.
31. **`CONNECT`** : la migration de `dashboard` le retire à `PUBLIC` **sur `dashboard` seule** ; `control_plane` le
    garde, que l'identité `breakglass` d'OpenBao utilise (007b-2 l'avait renvoyé à la deuxième base sans la nommer).
32. **Le `CHECK` d'`event` suit l'enum du contrat** : un test compare les valeurs du `CHECK` à celles
    d'`AccessLogEntry.event` dans `contracts/openapi-bff.yaml` ; une divergence ferait un 500 sur un événement neuf.
33. **Clé HMAC** (011a-2b) : reçue en `[]byte`, refusée sous 32 octets ; trois domaines, `session:` (sceau du cookie),
    `source:` et `email:` (sujets des compteurs, l'e-mail tapé par un inconnu n'est jamais gardé en clair).
34. **SMTP** (011a-2b) : STARTTLS si offert, vérifié par la CA interne ; `PLAIN` si `SMTP_USERNAME`, refusé sans TLS
    par l'adaptateur lui-même ; prouvé contre `axllent/mailpit:v1.31.4`.
35. **Cas d'usage** (011a-2b) : `Bootstrap`, `Login`, `UseAccessLink`, `Resolve` (ne prolonge pas la session : 08
    §8.3), `Activity`, `Logout`, `Me`, `ListAccessLog`, `DeliverNext`, `PurgeExpired`. Les boucles, le bootstrap au
    démarrage et l'adresse source derrière un proxy attendent 011a-3.
36. **Le 429 de `POST /auth/activity`** (humain, H) : déclaré au contrat, non servi ; la règle « au plus une fois par
    minute » est celle de la SPA. À dire dans la PR de 011a-3.

## Design arrêté

- **`internal/schema`** : `Grants{Service, Databases map[base]map[table][]privilège}` ;
  `ParseGrants(service, data, map[base]Classification)` (base inconnue refusée, table non classée dans **sa** base
  refusée) ; `LoadGrants(fsys, classifications)` ; `GrantsSQL(base, all)` (arbitrage 27).
- **`migrations`** : `Database(name) fs.FS` et `Grants(name) []byte` remplacent `ControlPlane()` et `Grants()` ;
  `Databases` liste `control_plane` et `dashboard`. Appelants : `cmd/migrate` (`control_plane` seul jusqu'en 011a-3),
  `cmd/catalog-apply` (`control_plane.sql` seul jusqu'en 011a-3), `pgtest`, `tools/grants`.
- **`migrations/dashboard/`** : `00001_dashboard.sql` (schéma, `REVOKE CONNECT … FROM PUBLIC`, tables de l'arbitrage
  29, porte de l'arbitrage 30 ; la descente rend `CONNECT` à `PUBLIC`), `audit.yaml`.
- **`contracts/grants/dashboard.yaml`** : `operators` `select, insert, update` ; `access_links` `select, insert,
  update` ; `sessions` et `login_attempt_counters` `select, insert, update, delete` ; `access_log` `select, insert`.
  011a-2b la resserre si ses requêtes en demandent moins.
- **`tools/grants`** : un fichier par base ; `migrations/grants/{control_plane,dashboard}.sql` générés.
- **`deploy/k8s/catalog-apply.yaml`** : la source `dashboard` du volume `role-secrets`.
- **`internal/platform/pgtest`** : `Empty` crée les deux bases, `ControlPlane` les migre toutes deux, `DB.In(base)`
  désigne l'autre base du même serveur.
- **Tests** : `schema` (déclaration à deux bases ; base inconnue ; table classée dans l'autre base ; fichier d'une base :
  rôles de l'union, droits de cette base seule, `CONNECT`, retrait par l'union) ; `migrations` paramétrés par base
  (classement, droits exacts, refus de ce qui n'est pas déclaré sous le rôle du service, retrait, montée-descente-
  remontée de `dashboard`, `CONNECT` de `PUBLIC` absent de `dashboard`, porte du journal d'accès, `CHECK` égal au
  contrat) ; garde de livraison inchangée, satisfaite par la source ajoutée.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité, tests creux, code en trop). Aucun bloquant ; 12 constats.

1. **Droits sur la base remis à zéro** : chaque fichier fait `REVOKE ALL ON DATABASE` pour chaque rôle de l'union,
   puis `GRANT CONNECT` à qui déclare une table de la base. Une dérive (`TEMP`, `CONNECT … WITH GRANT OPTION`)
   survivait au seul `REVOKE CREATE`. La dérive injectée par le test ne recouvre donc plus le `GRANT CONNECT` du
   fichier, que rien ne prouvait sur la base.
2. **`dashboard` fermée à `PUBLIC`** : la migration retire tous ses droits de base à `PUBLIC` (`TEMP` compris) ; la
   descente les rend.
3. **`access_log.target` en `text`** : le contrat type les identifiants en chaînes, et ceux des rôles (012) ne sont
   pas des uuid.
4. **Retraits** : trois `CHECK` qu'aucun test ne fait tomber (`attempts`, `failures`, `expires_at > created_at`), le
   commentaire SQL de la porte, une phrase du godoc de `GrantsSQL`, le helper `grantsFile`.
5. **Gardés tels quels** :
   - `control_plane` garde `CONNECT` et `TEMP` pour `PUBLIC` (arbitrage 31) : un secret du rôle `dashboard` fuité
     ouvre une session sans table métier.
   - Le rôle `dashboard` peut dater une ligne du journal : il ne fait qu'ajouter, et la purge reste à `migrator`.
   - Les appariements (`token_hash` et `expires_at`, défi et échéance) et la borne de 12 h relèvent du code de
     011a-2b, qui les teste.
   - La descente détruit le journal, comme toute première migration.
   - La liste des droits de `dashboard.yaml` est la donnée du design : le test des droits exacts la reflète, les
     requêtes de 011a-2b la resserrent.
   - Le retrait d'un rôle disparu se teste sur `control_plane` seul : le code est commun aux deux bases.
