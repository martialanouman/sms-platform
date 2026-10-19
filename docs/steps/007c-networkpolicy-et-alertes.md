# Step 007c — Livraison : NetworkPolicy générées, alertes

Plan : `docs/plan/socle.md` §007, quatrième des six PR. Spec : `docs/spec/02-architecture.md` §2.5 (deny par
défaut, flux générés depuis les allowlists et le catalogue ; port ops borné au kubelet et à Prometheus),
`docs/spec/10-exploitation.md` §10.1 (alertes) et §10.2. Décision : `docs/design/06-confiance-interne.md` §4.
Décisions humaines du 2026-10-19 : NetworkPolicy générées dans `deploy/k8s/`, sous `check-generated`, jamais
appliquées par `catalog-apply`. Règles : `.claude/rules/ci-deploy.md`, `.claude/rules/go-code.md`,
`.claude/rules/tests.md`. Mode : PR.

## Contexte

`deploy/k8s/` porte deux Jobs, `migrate` (Postgres) et `catalog-apply` (Postgres, Kafka). Aucun service
n'existe : aucune allowlist (`TLS_ALLOWED_CLIENTS`), aucun port ops déployé. Les pods n'ont aucun label.
go-gateway n'avait ni NetworkPolicy ni règle d'alerte.

Métriques émises par le code : `ops_metrics_gather_errors_total` seule (005d). Aucune métrique source du
tableau de 10.1 n'existe : lag de lane, garage, dead-letter, comptes opérateur, propagation, DLR, solde,
grand livre, OpenBao viennent des composants ; l'échéance des certificats vient de cert-manager (007d-1).
Plan : « seules entrent les règles dont la métrique source existe ; une règle entre avec le composant qui
émet sa métrique ».

**Terminé quand** (part de 007c) : un namespace où tout flux non généré est refusé ; chaque flux d'un
workload existant est dérivé de ses déclarations, sous `check-generated` ; les règles d'alerte sont
vérifiées et testées par `promtool` en CI.

## Arbitrages

Fable a tranché le 2026-10-19 (N1, N2, N4, A1, A2, A3 tenues ; N3, N5 amendées) :

- **N1 — Source** : les manifests, seule déclaration lisible (02 §2.5) ; le catalogue ne dit rien qu'un
  `KAFKA_BROKERS` ne dise.
- **N2 — Ce qui entre** : le `deny` d'entrée et de sortie du namespace, la sortie DNS de tout pod, la sortie
  de chaque workload vers Postgres et Kafka selon ses variables. Allowlists et port ops entrent avec le
  premier service, qui les exerce : un générateur sans entrée réelle ne s'est jamais vu échouer, et la règle
  du kubelet dépend du CNI. Séquencement, pas une dette : plan §007c et `.claude/rules/ci-deploy.md` le
  nomment. Ni sortie vers l'API Kubernetes (aucun pod ne monte de jeton), ni autre cible : chacune entre
  avec le workload qui la déclare.
- **N3 — Générateur** (amendée) : cœur `internal/netpol`, `Render(fs.FS) ([]byte, error)` ; racine
  `tools/netpol` (`os.DirFS`, `WriteFile`, comme `tools/grants`) ; `//go:generate` dans `doc.go`. Sortie dans
  un fichier plat, `deploy/k8s/networkpolicies.yaml` (constante du cœur), que la garde admet hors de la règle
  « un fichier par `cmd/` » si chacun de ses documents est une `NetworkPolicy`. Pas de `metadata.namespace` :
  posé à l'application (007d-2).
- **N4 — Labels** : `spec.template.metadata.labels` porte `app.kubernetes.io/name: <cmd>`, valeur égale au
  nom du fichier, exigé par la garde.
- **N5 — Cibles** (amendée) : sélecteurs de namespace et de pod, pas des ports seuls (un egress « 5432 vers
  partout » ne dérive d'aucune déclaration). Les magasins vivent dans leurs namespaces, hors du `deny` de la
  plateforme, dont les pods d'opérateur ne relèvent pas ; leurs politiques arrivent avec les briques
  (007d-1). Kafka : cluster, namespace et port dérivés de `KAFKA_BROKERS`
  (`<cluster>-kafka-bootstrap.<namespace>.svc:<port>`, refus sinon) ; le manifest de `catalog-apply` passe à
  `kafka-kafka-bootstrap.kafka.svc:9093`, le nom court ne résolvant que dans le namespace du Job. Postgres
  (l'URL vit dans un Secret) : namespace `postgres`, pods `cnpg.io/cluster: postgres`, 5432. DNS : `k8s-app:
  kube-dns` dans `kube-system`, 53 UDP et TCP. Un magasin managé (`ipBlock`) entre avec le déploiement qui
  l'exige (02 §2.6).
- **A1 — Outil** : `promtool check rules` et `test rules` dans `make check`. Réarbitré en TDD : `go run
  github.com/prometheus/prometheus/cmd/promtool@v0.315.0` échoue (le module porte des `replace`, que `go run`
  et `go install` refusent) ; promtool tourne donc par son image officielle épinglée par tag et digest
  (`prom/prometheus:v3.15.0@sha256:…`), dans le `Makefile` à côté de GoReleaser et grype. Docker est déjà
  requis (scan en CI, testcontainers en local). C1 amendée. Rejetés : l'archive de release (troisième
  mécanisme, une somme par plateforme), la bibliothèque Go (réécrire `promtool test rules`, tirer les `replace`).
- **A2 — Règles** : `MetricsGatheredPartially`, `increase(ops_metrics_gather_errors_total[10m]) > 0`, sans
  `for` ni `severity` (le routage entre avec Alertmanager), annotations sans interpolation. Légitime : sa
  métrique existe (005d), le tableau de 10.1 est une liste de départ ; un compteur sans règle est mort.
- **A3 — Garde** : chaque alerte de `deploy/alerts/<groupe>.yaml` a, dans `<groupe>_test.yaml`, un cas à
  `exp_alerts` non vide et un à `exp_alerts` vide ; garde Go dans `deploy/`.
- **Preuve réelle** : l'application par k3s relève de 007d-2 ; note pour 007d-1, k3s applique les
  NetworkPolicy par son contrôleur intégré, à ne pas désactiver.

## Design arrêté

- `internal/netpol` : `File = "networkpolicies.yaml"` ; `Render(fs.FS) ([]byte, error)` lit chaque manifest
  (`*.yaml` hors `File`) et rend, dans cet ordre :
  - `default-deny` : `podSelector: {}`, entrée et sortie ;
  - `dns` : sortie de tout pod vers `kube-system` / `k8s-app: kube-dns`, 53 UDP et TCP ;
  - une politique par workload qui déclare une dépendance, nommée comme lui, triée : sortie vers Postgres
    si `POSTGRES_URL`, vers chaque cluster Kafka de `KAFKA_BROKERS`, lues dans chaque document et chaque
    conteneur, init compris, sans doublon ; un élément vide de `KAFKA_BROKERS` ignoré comme le fait
    `config` ; refus d'un broker hors de la forme Strimzi ou d'un port hors de 1 à 65535, et d'un workload
    nommé `dns` ou `default-deny`.
- `tools/netpol` et `//go:generate go run ./tools/netpol` ; `deploy/k8s/networkpolicies.yaml` généré.
- `deploy/k8s/*.yaml` : label `app.kubernetes.io/name` sur chaque pod ; `KAFKA_BROKERS` qualifié.
- `deploy/guard_test.go` : `networkpolicies.yaml` admis hors des `cmd/`, de `NetworkPolicy` seules ; chaque
  pod porte son label. Cas faussés : un `Job` dans ce fichier, un pod sans label, un label d'un autre nom.
- `deploy/alerts/ops.yaml` et `ops_test.yaml` ; `Makefile` : `PROMTOOL` (image par digest, `-w /alerts`), cible
  `alerts` (`check rules`, `test rules`) dans `check` ; `deploy/alerts_test.go` : la garde A3, cas faussés (sans cas qui part, sans cas muet,
  alerte renommée).
- Docs : C1 (promtool), plan §007c (un flux entre avec le workload qui le déclare), `.claude/rules/ci-deploy.md`
  (un `cmd/` qui pose `TLS_ALLOWED_CLIENTS` ou sert `OPS_ADDR` étend le générateur dans sa PR).
- Tests du cœur : chaque règle (deny, DNS, Postgres, Kafka, plusieurs clusters, doublons, plusieurs
  documents, init containers, aucun besoin, broker ou port refusé, nom réservé) et l'ordre des politiques
  vus tomber ; `check-generated` tient le fichier des deux Jobs réels (une mutation du générateur le fait
  différer).
- Tests promtool : l'alerte part juste après une hausse et huit minutes après, se tait neuf minutes après
  (la fenêtre, ouverte à gauche, tenue à la minute près) et sur un compteur immobile.

## Revue

**Tour 1** (trois axes, aucun bloquant) :

- Retenus : la fenêtre `[10m]` de l'alerte n'était tenue que par sa borne basse (`[30m]` survivait) ;
  `internal/netpol` ne lisait pas les `initContainers` que la garde admet, et la lecture de plusieurs
  documents n'avait pas de test ; un élément vide de `KAFKA_BROKERS`, que `config` ignore, était refusé ; port non borné ; un
  workload nommé `dns` écrasait une politique fixe ; dédoublonnage de Kafka seul ; l'ordre des politiques
  et un `Job` après une vraie politique sans test ; groupes capturants superflus, branche `Atoi` morte,
  boucle de stabilité spéculative, `!ok` mort et ordre aléatoire dans la garde des alertes, message d'erreur
  incohérent, sous-shell dans le `Makefile`, territoire de `ci-deploy.md` sans `internal/netpol`.
- Écartés : un test de sortie exacte des deux Jobs réels recopierait le fichier que `check-generated` tient
  déjà (design amendé) ; `> 0` contre `>= 0.5` : un compteur entier n'a pas d'accroissement entre les deux ;
  un échec au tout premier scrape d'un pod (série née à 1) reste muet, un échec qui dure incrémente et part.

**Tour 2** (un axe sur les correctifs, trois mineurs, aucun bloquant) : borne haute de la fenêtre tenue à
trois minutes près (le cas muet passe de 13 à 10 minutes) ; `[0-9]{1,5}` redondant avec la borne du port ;
fermeture recréée à chaque document.
