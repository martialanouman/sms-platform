# Step 006a — Données : goose, `control_plane`, trigger d'audit, classement

Plan : `docs/plan/socle.md` §006, première des deux PR. Règles : `.claude/rules/migrations.md`,
`.claude/rules/tests.md` (Intégration). Mode : PR.

## Contexte

go-gateway migrait par golang-migrate, dans un seul schéma possédé par le rôle applicatif ;
`audit_log` s'écrivait en deux temps (intention, puis issue), et son immuabilité reposait sur un
trigger et sur un `REVOKE` au propriétaire (migrations 0015, 0020, 0021). Les décisions 5, 6 et 7
veulent un propriétaire distinct (`migrator`), un `audit_log` écrit une fois, et un `audit_changes`
rempli par trigger, qui refuse une écriture sans acteur et notifie au commit.

**Terminé quand** (part de 006a) : une écriture sans `app.actor` échoue ; une table non classée fait
échouer la garde ; up/down/up passe sous `migrator`.

## Arbitrages

Fable a tranché le 2026-10-18 ; l'humain a tranché le partitionnement le même jour.

- **Découpage** : 006a (ici), puis 006b (rôles, GRANT générés), qui lit le classement d'ici.
- **Bases** : `control_plane` seule. `message_state`, `dashboard`, `cdr` et le câblage goose de
  ClickHouse arrivent avec leur premier composant ; le `Cuttable` Postgres avec son premier
  consommateur, la bibliothèque d'instantané. Amendé : plan §005 et §006. Déplacements, pas des dettes.
- **Exécution** : goose v3 en bibliothèque (`Provider`), SQL embarqué par `embed.FS`. Aucun
  `cmd/migrate` : le Job arrive en 007 avec GoReleaser et `deploy/k8s/`. La table de version de goose
  reste dans `public`.
- **Partitionnement** (humain, sur recommandation de Fable) : **aucun**. La purge est un `DELETE` sous
  plancher (ADR-0018), le volume de quelques dizaines de lignes par jour, et une ligne tombée dans
  `DEFAULT` y resterait coincée par l'immuabilité. Amendé : spec 07 §7.5, design 05 §4.4.
- **`audit_log`** : colonnes du contrat `AuditEntry`, écriture unique ; `outcome` sans `CHECK` (le
  contrat n'énumère pas : une seconde liste en base serait tenue par convention). PK `(at, id)`, la clé
  keyset de la lecture. Aucun autre index : ils arrivent avec le lecteur.
- **`audit_changes`** : `table_name` (`table` est réservé ; le champ du contrat reste `table`), `op` en
  miroir de l'enum du contrat, PK `(at, id)` (`now()` est constant dans une transaction, `uuidv7`
  départage).
- **Immuabilité** : une fonction pour les deux tables ; `UPDATE` et `TRUNCATE` refusés, `DELETE` admis
  sous `audit_log.purge = on` et au-delà de `8760 hours` ; 42501. La danse `REVOKE`/`GRANT` au
  propriétaire disparaît : le propriétaire est `migrator`, les services n'ont que leurs GRANT.
- **Trigger de diff** : `SECURITY DEFINER`, `search_path` figé, tout qualifié, pour qu'aucun service
  n'ait d'`INSERT` sur `audit_changes`. Acteur absent ou vide : 22023 (distinct du 42501 d'un droit
  manquant). `pk` est toujours l'objet jsonb des colonnes de la clé primaire, et la notification
  `config` porte `<table>:<pk>` sous ce seul format. Le trigger par instruction (`<table>:*`) et le
  canal `optout` arrivent avec leurs tables.
- **Classement** : `migrations/control_plane/audit.yaml`, lu hors base parce que 006b génère les GRANT
  sans base. `COMMENT ON TABLE` écarté.

## Design arrêté

- `migrations/control_plane/00001_audit.sql` : schéma `control_plane` ; `audit_log` et
  `audit_changes` ; `control_plane.audit_append_only()` sur les deux (row `UPDATE`/`DELETE`,
  statement `TRUNCATE`) ; `control_plane.audit_capture()`, prête à poser sur chaque table `diff`.
- `migrations/control_plane/audit.yaml` : `table → {audit: diff|event|exempt, reason}`, `reason`
  obligatoire hors `diff` ; `audit_log`, `audit_changes` en `exempt`.
- `migrations` (paquet Go) : `ControlPlane fs.FS`, les fichiers de la base.
- `internal/schema` (cœur) : `ParseClassification` refuse un classement inconnu et une raison manquante.
- `internal/platform/pgtest` : un `postgres:18` par processus, le rôle `migrator` non superuser
  propriétaire de la base, les migrations appliquées par lui ; de quoi s'y connecter en superuser et en
  `migrator`.
- Tests d'intégration (`migrations`) : up/down/up ; sur une table sonde portant `audit_capture`, une
  écriture sans acteur échoue en 22023, une écriture attribuée laisse avant/après sans colonne scellée
  et notifie `config` au commit ; `UPDATE`/`DELETE`/`TRUNCATE` refusés, la porte de purge s'ouvre
  au-delà du plancher seulement ; la garde : toute table de `control_plane` est classée, toute entrée
  existe, une `diff` porte `audit_capture` et une clé primaire, une autre ne la porte pas.
- `go.mod` : `pressly/goose/v3`, `jackc/pgx/v5` en directe.
