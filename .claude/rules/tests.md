---
paths:
  - "**/*_test.go"
  - "**/*.feature"
---

# Tests

- **Rouge lu d'abord**, échouant pour la bonne raison : « symbole inexistant » le prouve, « connexion
  refusée » ne prouve rien. **Aucun vert sans mutation vue tomber.**
- **Muter à la couche où vit le défaut** : le code, mais aussi la **donnée d'entrée** et le **câblage**.
  Muter le SQL généré, pas `queries/*.sql`. Copier avant de muter, vérifier la restauration par `cmp` ;
  jamais `git stash` pour annuler.
- **Répéter** avant de conclure : `-count=10`, 25 si concurrent.
- Toute étape de pipeline neuve a un test qui prouve qu'elle **ne journalise pas le corps** (I1).

## Ce qui rend un test creux

- un aller-retour qui rejoue la fonction testée ;
- une fixture littérale (cassée sous `-count>1`) ou un champ laissé à zéro ;
- une garde testée sur une copie du câblage, pas le câblage réel ;
- deux filtres qui se recouvrent : aucun des deux n'est prouvé ;
- un test de backoff à planchers seuls, un `sync.Once` dans un test de course, une comparaison à la
  constante qu'on teste ;
- un ordonnancement supposé : utiliser une barrière ;
- un test de `cmd/` qui contourne `Validate` ; un boot que rien ne construit en test est cassé ;
- une métrique au catalogue n'est pas exposée pour autant.

Quand un test accuse, **suspecter la fixture avant le code** : un double doit modéliser le contrat.

## Intégration

- testcontainers-go sur les vraies dépendances. **Un test qui saute en CI échoue** (`ciguard`).
- En local (OrbStack), exporter dans **chaque** commande `DOCKER_HOST` et
  `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE`, sinon tout saute en silence ; lire `-v` (`--- SKIP`) avant de
  croire un vert. **Jamais `TESTCONTAINERS_RYUK_DISABLED`.**
- Conteneur par processus : un littéral partagé casse le second passage.
- Couper une dépendance : `Cuttable` et `tcpproxy` ; lire l'effet par un second client non coupé.

## Mesure de charge

Débit **traversant**, hôte au repos, jamais deux mesures en parallèle, image réellement déployée
vérifiée. Un banc sans la corrélation DLR mesure 1,6× la production ; la co-résidence des dépendances
fausse le plafond ; un littéral laissé à zéro fausse tout.
