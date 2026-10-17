---
paths:
  - "docs/plan/**"
  - "docs/steps/**"
---

# Steps et dettes

- **Le numéro est l'ordre d'exécution**, depuis 001. Une step ne dépend que de numéros plus petits.
- `docs/steps/NNN-slug.md` porte le contexte, les arbitrages et `## Design arrêté`, **commité avant la
  première ligne de code**. Un correctif de revue repasse par les arbitrages et le design.
- Déroulé : design → TDD → mutation → revue en lecture seule (un axe cherche le code en trop) → coupe →
  DoD → PR. Au troisième tour de revue sans convergence, l'humain tranche.
- **Différer, c'est ouvrir une issue `debt`** dans la même PR (ce qu'on a fait à la place, pourquoi, ce
  qu'il en coûte, le déclencheur, le porteur), et marquer `debt: #N` dans le code qui porte le report.
  Payer la dette, c'est retirer la marque et fermer l'issue dans la même PR. `go-live` en plus si elle
  bloque la mise en production.
- Une phrase de step qui qualifie du code existant de faux ou d'obsolète est **vérifiable** : l'ouvrir
  avant de l'écrire.
