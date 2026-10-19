---
paths:
  - "migrations/**"
  - "internal/**/queries/**"
---

# Migrations et schéma

goose pour Postgres et ClickHouse, un répertoire par base. **Une table neuve** se classe pour l'audit
(diff, événement seul ou exemptée avec raison) ; dans `control_plane`, le classement vit dans
`migrations/control_plane/audit.yaml`, et une table `diff` porte le trigger `control_plane.audit_capture()`
(sur les trois écritures, sans `UPDATE OF` ni `WHEN`) et une clé primaire. Elle entre aussi dans la déclaration de
GRANT de chaque service qui la lit ; les gardes refusent sinon. Une migration reste compatible avec la version précédente du
code pendant un déploiement progressif.

## Postgres

- `PARTITION OF` prend ACCESS EXCLUSIVE sur le parent : créer puis `ATTACH`.
- `'365 days'` suit l'heure d'été et `'1 year'` vaut 366 jours en bissextile : compter en `hours`.
- Un `REVOKE` est invisible sous superuser : tester sous le rôle du service.
- Verrouiller une ligne absente fait du 40P01 sous course.
- Les compteurs de pgxpool ne montrent pas la famine ; `MaxConns` borne une concurrence, pas un débit.
- sqlc 1.30 : qualifier les tables par leur schéma.
- Deux PR parallèles prennent le même numéro de migration : vérifier `main` avant de pousser.

## ClickHouse

- **Une instruction par fichier** : le découpeur casse sur un `;` en commentaire.
- Pagination keyset sur `DateTime64` : comparer en `toUnixTimestamp64Milli`.
- Un seul propriétaire de la rétention : une TTL de table concurrente d'un archivage détruit l'archive ;
  `engine_file_truncate_on_insert` tronque avant d'écrire.
