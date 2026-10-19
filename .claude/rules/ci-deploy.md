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
- **k3s (kube-router)** n'admet un pod neuf qu'après quelques secondes : sa sortie tombe d'abord sous
  `default-deny` (REJECT, « connection refused »). Un `cmd/` qui dial Postgres au démarrage passe par
  `pgdial.Retry` ; franz-go rejoue de lui-même. Un `/readyz` ne dépend jamais d'un dial unique au démarrage.
- La métadonnée Kafka est éventuellement cohérente par broker : `catalog-apply` échoue bruyamment sur une
  lecture périmée et se rejoue entier (`backoffLimit: 2`), jamais par une retentative partielle de
  l'adaptateur.
- **Une alerte** vit dans `deploy/alerts/<groupe>.yaml`, avec un cas qui la fait partir et un qui la laisse
  muette dans `<groupe>_test.yaml` ; elle entre avec le composant qui émet sa métrique.
- `exit 139` d'un conteneur en CI vient de `fs.aio-max-nr`, jamais de la mémoire : faire parler le
  conteneur mort.
- minio a disparu de Docker Hub : RustFS le remplace.
- Le cache golangci est partagé entre worktrees.
- **PromQL** : une expression à deux opérandes nomme ses labels des deux côtés, sinon elle n'apparie
  rien et ne part jamais ; une jauge muette se couvre par `absent(g >= 0)`. `promtool test rules` le
  prouve.
