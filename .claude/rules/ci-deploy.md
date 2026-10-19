---
paths:
  - ".github/**"
  - "Makefile"
  - ".goreleaser*"
  - "deploy/**"
  - "internal/netpol/**"
---

# CI, livraison, alertes

- **Un job agrégateur requis** ; actions épinglées par SHA ; un test qui saute échoue.
- **Chaque `cmd/` neuf** entre dans GoReleaser et `deploy/k8s/` dans la même PR, son pod porte
  `app.kubernetes.io/name: <cmd>` ; `go generate` régénère `deploy/k8s/networkpolicies.yaml`. Un `cmd/` qui
  pose `TLS_ALLOWED_CLIENTS`, sert `OPS_ADDR` ou lit une dépendance neuve étend `internal/netpol` dans sa PR.
- **Déploiement de l'environnement de test** : la VM tire le dernier tag de release (`deploy/test-env/sync.sh`,
  timer systemd) et joue `deploy/test-env/deploy.sh <tag>`. Un `cmd/` neuf y entre à sa place dans la séquence
  (`migrate`, `catalog-apply`, puis les services ; la garde exige chaque `cmd/` une fois dans la boucle des
  Jobs, pas l'ordre, et sera revue avec le premier service) ; chaque Job porte `activeDeadlineSeconds`.
  `sync.sh` ne déploie qu'un tag ancêtre de `main` ; il tourne depuis sa copie, que seul `bootstrap.sh`
  rejoué répare (`docs/steps/007d-2b-deploiement-par-tag.md`, « Reprise »).
- **k3s (kube-router)** n'admet un pod neuf qu'après quelques secondes : sa sortie tombe d'abord sous
  `default-deny` (REJECT, « connection refused »). Un `cmd/` qui dial Postgres au démarrage passe par
  `pgdial.Retry` ; franz-go rejoue de lui-même. Un `/readyz` ne dépend jamais d'un dial unique au démarrage.
- La métadonnée Kafka est éventuellement cohérente par broker : `catalog-apply` échoue bruyamment sur une
  lecture périmée et se rejoue entier (`backoffLimit: 2`), jamais par une retentative partielle de
  l'adaptateur.
- **Qui demande un certificat** : cert-manager n'agrège pas `certificates` aux rôles `edit` et `admin` ;
  le premier compte ou outil qui reçoit `edit` ou `admin` sur un namespace apporte approver-policy (ou un
  `Issuer` par namespace) dans sa PR. La clé racine ne quitte jamais `cert-manager`.
- **Release** : le job `release` de `ci.yml` pose `vX.Y.Z` depuis les commits conventionnels de `main` ; le
  titre et le corps squashés sont ceux de la PR (réglage du dépôt) : `pr-title` valide le titre ; le corps ne
  l'est pas, et une ligne qui y commence par `BREAKING CHANGE:` monte la version. La liste
  des types vit deux fois, dans `pr-title` et dans `internal/release` : on change l'une avec l'autre. Un
  ruleset de tags devra exempter `github-actions`, qui crée le tag par la release.
- **Une image de brique** est épinglée par la version de son chart ou de sa release ; par digest quand le
  tag n'est pas une release (Postgres).
- **Une alerte** vit dans `deploy/alerts/<groupe>.yaml`, avec un cas qui la fait partir et un qui la laisse
  muette dans `<groupe>_test.yaml` ; elle entre avec le composant qui émet sa métrique.
- `exit 139` d'un conteneur en CI vient de `fs.aio-max-nr`, jamais de la mémoire : faire parler le
  conteneur mort.
- minio a disparu de Docker Hub : RustFS le remplace.
- Le cache golangci est partagé entre worktrees.
- **Cache Go de la CI** : `check` et `release` le clent sur `go.sum` et le `Makefile`, où vivent les versions de
  GoReleaser et grype ; sinon `codeql`, premier fini, enregistre la clé commune sans eux, et ils se recompilent
  à chaque run (6 min).
- **PromQL** : une expression à deux opérandes nomme ses labels des deux côtés, sinon elle n'apparie
  rien et ne part jamais ; une jauge muette se couvre par `absent(g >= 0)`. `promtool test rules` le
  prouve.
