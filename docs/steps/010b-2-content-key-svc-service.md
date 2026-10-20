# Step 010b-2 — `content-key-svc` : service et déploiement

Plan : `docs/plan/composants.md` §010, seconde des deux PR de 010b. Arbitrages : `docs/steps/010b-1-content-key-svc-coeur.md`
(arbitrages 1, 12 et 13 ; tours de revue, points « pour 010b-2 »), Fable, 2026-10-20. Spec : `docs/spec/02-architecture.md`
§2.2, §2.5 ; `docs/spec/10-exploitation.md` §10.2. Décision : `docs/design/06-confiance-interne.md` §4, §6. ADR : 0011,
0016. Règles : `.claude/rules/ci-deploy.md`, `.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 010b-1 (#126) a livré `internal/contentkey` et ses adaptateurs OpenBao et gRPC ; `Register` n'installe pas
  l'intercepteur `Authorize`.
- Aucun service (non-Job) n'existe : la garde de `deploy/` refuse tout autre kind que `Job` et
  `HorizontalPodAutoscaler`, et `deploy.sh` n'a qu'une boucle de Jobs.
- `internal/netpol` ne génère que des sorties (Postgres, Kafka) ; OpenBao admet déjà l'entrée 8200 des pods
  `content-key-svc` (010a).
- Dettes du territoire : #26 (allowlist du manifest), #43 (refus de handshake hors slog), #76 (pod bloqué sans
  Secret TLS), #93 (exportateur OTLP), #70 et #122 (reportées en 010b-1).

**Terminé quand** : la release déploie `content-key-svc` sur l'environnement de test et son `/readyz` y répond ; un
secret scellé ne s'ouvre que par lui ; seuls ses appelants admis l'atteignent, et seulement par leurs méthodes.

## Arbitrages

Arrêtés en 010b-1 (Fable), rappelés ici ; les points 9 à 12 sont tranchés par l'agent, sans contredire la spec.

1. **`/readyz` sans dépendance** à OpenBao : un OpenBao scellé ne bloque ni le rollout ni le timer de déploiement ;
   l'indisponibilité se voit par `Unavailable` et l'alerte `OpenBaoSealed`.
2. **Démarrage refusé** sur `GRPC_ADDR` vide, `OPENBAO_ADDR` qui n'est pas `https://`, et `TLS_ALLOWED_CLIENTS` qui
   n'est pas exactement l'ensemble des appelants de `contentkeygrpc.Callers`. Les deux variables n'ont pas de défaut :
   le générateur de NetworkPolicy ne lit que les manifests.
3. **NetworkPolicy** : sortie dérivée d'`OPENBAO_ADDR` (`https://<nom>.<ns>.svc:<port>`, refus sinon) vers les pods
   `app.kubernetes.io/name: <nom>` de `<ns>` ; entrée sur le port de `GRPC_ADDR` depuis les pods
   `app.kubernetes.io/name: <appelant>` de `sms` pour chaque appelant de `TLS_ALLOWED_CLIENTS`, refus si l'un est posé
   sans l'autre. Pas d'entrée sur le port ops : Prometheus entre en 011a ; kube-router admet les sondes du kubelet
   (`--src-type LOCAL`), à prouver sur le VPS. Un CNI managé qui ne le ferait pas exigerait un `ipBlock` du nœud.
4. **Manifest** : ServiceAccount, Deployment (une réplique, pas de PDB, 10 §10.2 ne le liste pas),
   `automountServiceAccountToken: false` et jeton projeté (audience `openbao`, 3 600 s), Service ClusterIP 7002,
   `terminationGracePeriodSeconds: 125` (`DRAIN_DELAY` + `DRAIN_BUDGET` + `SHUTDOWN_TIMEOUT` aux défauts), sondes
   `/readyz` et `/healthz` sur le port ops. Certificat `sms/content-key-svc-tls` (SAN `content-key-svc`, `server auth`).
5. **`deploy.sh`** : une boucle des services après celle des Jobs, `kubectl rollout status` borné. La garde exige
   chaque `cmd/` une fois dans l'union des deux boucles, un Job dans celle des Jobs, un Deployment dans celle des
   services, les Jobs d'abord ; elle admet Deployment (règles de pod des Jobs, plus la grâce et la sonde de
   disponibilité), Service (nom du `cmd/`, port de `GRPC_ADDR`) et ServiceAccount (nom du `cmd/`).
6. **#26** : la garde compare `TLS_ALLOWED_CLIENTS` du manifest aux clés de `contentkeygrpc.Callers` ; la moitié
   « gardes de migration `down` » reste ouverte, issue renommée.
7. **#43, moitié gRPC** : `grpclog` vers slog ; un refus de handshake sort en JSON. La moitié `net/http` reste ouverte.
8. **#93** : reportée, déclencheur réécrit (brique collecteur et premier service du chemin des messages) ; #76 :
   reportée, `rollout status` rend le blocage borné et bruyant.
9. **Redirections refusées par l'adaptateur** : `openbao.New` copie le client et refuse toute redirection, sinon
   `X-Vault-Token` et le JWT suivraient un 307 ; la propriété vit avec qui en dépend.
10. **Client OpenBao sans certificat client** : racines de `TLS_CA_FILE`, TLS 1.3 ; le listener d'OpenBao demande un
    certificat sans l'exiger, et le certificat du service n'est que `server auth`.
11. **Adaptateur `grpclog` dans `internal/platform/observability`** : tout serveur gRPC en aura besoin. Info de grpc
    en Debug (bavard), Warning en Warn, Error en Error, Fatal en Error puis sortie ; verbosité 0.
12. **Arrêt du serveur gRPC** : `GracefulStop` à l'annulation, borné par `SHUTDOWN_TIMEOUT`, puis `Stop`.

## Design arrêté

- **`internal/platform/config`** : `GRPC_ADDR` et `OPENBAO_ADDR`, sans défaut.
- **`internal/platform/observability`** : `GRPCLogger(*slog.Logger) grpclog.LoggerV2`.
- **`internal/contentkey/openbao`** : `New` refuse les redirections.
- **`cmd/content-key-svc/main.go`** : `run(ctx, infra, log)` valide (arbitrage 2), installe `grpclog`, compose
  Transit, serveur gRPC (`Authorize` compris), ops et superviseur (ops, puis gRPC ; `ops.Drain` au drain).
- **`internal/netpol`** : `Ingress` et la sortie d'`OPENBAO_ADDR` (arbitrage 3) ; `networkpolicies.yaml` régénéré.
- **`deploy/k8s/content-key-svc.yaml`**, **`deploy/bricks/ca.yaml`** (certificat), **`deploy/test-env/deploy.sh`**
  (attente du certificat, boucle des services), **`.goreleaser.yaml`**, **`deploy/guard_test.go`** (arbitrages 5 et 6,
  un cas faussé par règle neuve).
- **`.claude/rules/ci-deploy.md`** : les deux boucles ; l'entrée ops avec Prometheus.
- **Tests** : `cmd/` sur le câblage réel (démarrage refusé dans chaque cas ; `/readyz` 200 sans OpenBao ; `Open`
  refusé à `admin-api-svc` par l'intercepteur, `Seal` admis jusqu'à la KMS ; refus de handshake en JSON ; drain) ;
  redirection refusée ; `netpol` ; garde.
- **Preuve sur `contabo169`** : image construite depuis la branche ; `Seal` depuis un pod au SAN `admin-api-svc`
  (certificat ad hoc) ; `Open` refusé ; refus au handshake d'un autre SAN ; `connection refused` depuis un pod non
  admis ; `/readyz` 200 OpenBao scellé, `Seal` en `Unavailable` ; sondes du kubelet admises ; refus de handshake dans
  les journaux JSON.
- **Après la release** (geste humain) : rendre public le paquet GHCR `content-key-svc`.
