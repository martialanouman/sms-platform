---
paths:
  - "contracts/topics.yaml"
  - "internal/lane/**"
  - "internal/storage/kafka/**"
---

# Topics et lanes

- **Tout topic et tout groupe est déclaré** dans `contracts/topics.yaml` avant d'être produit ou
  consommé ; une lane qui signale l'échec d'une dépendance non déclarée, ou d'un autre mode, s'arrête.
- **Une dead-letter dès le premier jour.** `start: latest` et `fail_open` exigent une justification
  écrite.
- Le nombre de partitions d'un topic `order: per_key` est **figé** : en ajouter redistribue les clés.
- Un topic qui porte le corps est `clear_bounded`, rétention ≤ 96 h.
- `origin_ts` est posé par le premier producteur et **ne change jamais** : l'âge maximal se mesure
  depuis lui, à travers garages, dead-letters et rejeux (I10).
- Une issue inconnue est `Transient`, jamais `Refuse` : un message refusé à tort ne revient pas.
- Un service consommant par groupe : `maxReplicas` ≤ partitions du plus petit topic consommé.
