---
paths:
  - "contracts/**"
---

# Contrats

**Le contrat se déclare avant l'implémentation**, et le code s'y conforme. La liste des scopes,
les types Go du BFF et les types TypeScript de la SPA en sont générés ; `check-generated` échoue sur un écart.

- `openapi-public.yaml` : une rupture classée `ERR` par `oasdiff` exige une version majeure — les
  clients ne se déploient pas avec nous.
- `openapi-admin.yaml` : **chaque opération déclare exactement un scope**. Une opération sans scope est
  publique (huma n'applique pas le `security:` global). Toute opération déclarée est servie.
- `proto/` : `buf breaking` bloquant — un record en rétention ou un pod de la version précédente doit
  rester lisible.
- **OpenAPI** : 3.1, **en anglais**, descriptions comprises ; aucun commentaire YAML. Nullable par
  `type: [T, "null"]` ; erreur plate `{ code, message, errors[] }`, le statut HTTP sur la ligne de
  statut seule ; un MSISDN n'entre jamais dans une URL ni un curseur. Public : `operationId`
  kebab-case, champs et paramètres snake_case.
- Une opération pas encore servie publie déjà ses schémas : durcir un champ ensuite est une rupture.
- `make generate` efface un commentaire écrit à la main dans un fichier généré.
- Deux PR parallèles prennent la même version : vérifier `main` avant de pousser.
