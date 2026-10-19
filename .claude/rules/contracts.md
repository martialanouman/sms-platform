---
paths:
  - "contracts/**"
---

# Contrats

**Le contrat se déclare avant l'implémentation**, et le code s'y conforme. La liste des scopes et des permissions en
est générée, comme les types Go du BFF et TypeScript de la SPA avec leur composant ; `check-generated` échoue sur un écart.

- `openapi-public.yaml` : une rupture classée `ERR` par `oasdiff` exige une version majeure — les
  clients ne se déploient pas avec nous.
- `openapi-admin.yaml` : **chaque opération déclare exactement un scope**. Une opération sans scope est
  publique (huma n'applique pas le `security:` global). Toute opération déclarée est servie.
- `openapi-bff.yaml` : chaque opération déclare `security` — `[]` pour les seules publiques, sinon la
  session et au plus une permission (BFF ou scope Admin). Aucun chemin sous `/gw`, le proxy.
- `grants/<service>.yaml` : ce que le service lit ou écrit dans `control_plane`, table par table, en
  `select`, `insert`, `update`, `delete` seuls ; une table non classée est refusée. Le rôle est le nom du
  service, `-` en `_`. Régénérer `migrations/grants/control_plane.sql` ; rien d'autre n'accorde un droit.
- `proto/` : `buf breaking` bloquant — un record en rétention ou un pod de la version précédente doit
  rester lisible.
- **OpenAPI** : 3.1, **en anglais**, descriptions comprises ; aucun commentaire YAML. Nullable par
  `type: [T, "null"]` ; erreur plate `{ code, message, errors[] }`, le statut HTTP sur la ligne de
  statut seule ; un MSISDN n'entre jamais dans une URL ni un curseur. Public et Admin :
  `operationId` kebab-case, champs et paramètres snake_case. BFF : `operationId`, champs et paramètres
  camelCase.
- Une opération pas encore servie publie déjà ses schémas : durcir un champ ensuite est une rupture.
- `make generate` efface un commentaire écrit à la main dans un fichier généré.
- Deux PR parallèles prennent la même version : vérifier `main` avant de pousser.
