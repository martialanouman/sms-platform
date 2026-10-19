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
  journaux (ADR-0018). Les autres arrivent avec leur composant et leurs tables.
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
- Un rôle retiré des déclarations garde ses droits jusqu'à ce que `catalog-apply` les retire : c'est
  l'affaire de 007, qui applique en production.

## Design arrêté

- `contracts/grants/admin-api-svc.yaml` : `control_plane: {audit_log: [select, insert, delete],
  audit_changes: [select, delete]}`.
- `internal/schema` : `ParseGrants(service, data, classification)` refuse un nom de service invalide,
  une base autre que `control_plane`, une table non classée, un privilège hors des quatre ;
  `Role(service)` ; `GrantsSQL([]Grants)` rend le fichier, rôles triés.
- `tools/grants` : lit les déclarations et `audit.yaml`, écrit `migrations/grants/control_plane.sql`
  (`//go:generate` dans `doc.go`).
- `internal/platform/pgtest` : `migrator` avec `CREATEROLE`.
- Tests d'intégration (`migrations`) : le fichier commité s'applique deux fois sous `migrator` ; les
  droits sur les tables de `control_plane` (`aclexplode`, `PUBLIC` compris, hors propriétaire) égalent
  exactement les déclarations ; chaque rôle a `USAGE` ; sous `admin_api_svc`, un `UPDATE` d'`audit_log`
  et la lecture d'une table non déclarée rendent 42501, ses droits déclarés passent.
- `.claude/rules/contracts.md` : le territoire `contracts/grants/`.
