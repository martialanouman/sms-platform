# Step 006b — Données : rôles, GRANT générés

Plan : `docs/plan/socle.md` §006, seconde des deux PR. Règles : `.claude/rules/migrations.md`,
`.claude/rules/contracts.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

go-gateway n'avait qu'un rôle Postgres, propriétaire de tout. La décision 6 (§5.1) veut un rôle par
service, qui n'a que les GRANT des tables qu'il déclare, générés depuis un fichier par service et le
classement d'audit de 006a ; `migrator` reste seul propriétaire.

**Terminé quand** (part de 006b) : un rôle de service ne lit rien hors de sa déclaration, testé sous ce
rôle, non superuser ; une garde refuse un droit non déclaré.

## Arbitrages

Fable a tranché le 2026-10-18 (arbitrage A7 de la step 006) :

- **Déclarations** : `contracts/grants/<service>.yaml`, une par service, base → table → privilèges. La
  seule aujourd'hui : `admin-api-svc`, écrivain d'`audit_log` (spec 07 §7.5) et purgeur des deux
  journaux (spec 05 §5.3, ADR-0018). Les autres arrivent avec leur composant et leurs tables.
- **Rôles** : nom du service, `-` remplacé par `_` (`admin_api_svc`), jamais déclaré à la main.
- **Génération** : un cœur dans `internal/schema` (lecture, validation contre `audit.yaml`, rendu SQL),
  une racine `tools/grants`, sortie commitée `migrations/grants/control_plane.sql`, sous `go generate`,
  donc sous `check-generated`. Hors du répertoire goose : goose refuse un `.sql` sans version.
- **SQL** : par rôle, `CREATE ROLE … NOLOGIN` idempotent (`LOGIN` et secret au déploiement, 007),
  `USAGE` sur le schéma, `REVOKE ALL ON ALL TABLES` puis les GRANT déclarés ; sans le `REVOKE`, l'état
  dérive de la déclaration et la garde « exactement » ment. Appliqué par le test ici, par
  `catalog-apply` en 007. `migrator` reçoit `CREATEROLE`.
- **Garde** : après migrations et SQL généré, les droits de chaque rôle sur `control_plane` sont
  exactement sa déclaration ; une déclaration vers une table non classée échoue au générateur ; sous le
  rôle de service, avec `LOGIN` posé par le test, un droit non déclaré rend 42501.

Tranché ici, dans le cadre de A7 :

- **Une transaction** enveloppe le fichier généré : entre le `REVOKE` et le `GRANT`, un service en
  marche perdrait ses droits.
- **Privilèges déclarables** : `select`, `insert`, `update`, `delete`. `truncate` contourne le trigger
  de capture (revue de 006a) ; `references` et `trigger` n'ont aucun usage de service.
- Un rôle retiré des déclarations garde ses droits jusqu'à ce que `catalog-apply` les retire ; le
  `LOGIN`, le secret et l'application unique sur la première erreur sont aussi à 007 (plan §007).

## Design arrêté

- `contracts/grants/admin-api-svc.yaml` : `control_plane: {audit_log: [select, insert, delete],
  audit_changes: [select, delete]}`.
- `internal/schema` : `ParseGrants(service, data, classification)` refuse un nom de service invalide,
  une base autre que `control_plane`, une table non classée, un privilège hors des quatre ;
  `Role(service)` ; `LoadGrants(fs.FS, classification)` ; `GrantsSQL([]Grants)` rend le fichier, rôles
  triés.
- `tools/grants` : lit les déclarations et `audit.yaml`, écrit `migrations/grants/control_plane.sql`
  (`//go:generate` dans `doc.go`).
- `internal/platform/pgtest` : `migrator` avec `CREATEROLE`.
- Tests d'intégration (`migrations`) : le fichier commité s'applique deux fois sous `migrator` ; les
  droits sur les tables de `control_plane` (`aclexplode`, `PUBLIC` compris, hors propriétaire) égalent
  exactement les déclarations ; chaque rôle a `USAGE` ; sous `admin_api_svc`, un `UPDATE` d'`audit_log`
  et la lecture d'une table non déclarée rendent 42501, ses droits déclarés passent.
- `.claude/rules/contracts.md` : le territoire `contracts/grants/`.

## Revue

Trois axes en lecture seule. Correctifs :

- **Adoption d'un rôle existant** : un service nommé `pg-signal-backend` ou `migrator` faisait accorder
  les droits à un rôle prédéfini, ou retirer les siens au propriétaire. Le parseur refuse le préfixe
  `pg-` et un nom de plus de 63 octets ; le SQL généré refuse un rôle existant qui n'est pas simple
  (superuser, `CREATEROLE`, `CREATEDB`, `BYPASSRLS`, réplication, membre d'un rôle). Testé, cas par cas.
- **Schéma** : `REVOKE ALL ON SCHEMA` avant `USAGE` ; un `CREATE` dérivé laissait le service créer une
  table à lui, qui faisait ensuite échouer tout le fichier.
- **Garde** : la grant option, les droits de colonne et toute table qui n'appartient pas à `migrator`
  entrent dans la comparaison ; la dérive du test les pose tous.
- **Tests** : le fichier commité est appliqué, et non plus le rendu en mémoire ; le refus d'`UPDATE` porte sur
  une requête sans ligne (le trigger d'immuabilité rend aussi 42501) ; chaque droit déclaré est exercé.
- **Second tour** : un rôle qui a des membres, par héritage ou par `SET`, est refusé lui aussi (le lien
  `ADMIN OPTION` que Postgres 16+ donne à `migrator` sur les rôles qu'il crée reste admis) ; `CREATE` sur
  la base est révoqué et gardé, un service créait sinon un schéma hors de toute garde.
- Code en trop retiré : le type `Privilege` et ses constantes exportées, la double map, le `MkdirAll`,
  `TestRole`. Un privilège déclaré deux fois est refusé.

Notes : `CREATEROLE` donne à `migrator` l'`ADMIN OPTION` sur les rôles qu'il crée (Postgres 16+) ; ses
identifiants valent ceux de chaque service, coût déjà accepté par l'arbitrage. Les noms de table du SQL
viennent d'`audit.yaml`, lié aux tables réelles par la garde de 006a.
