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
11. **Adaptateur `grpclog` dans `internal/platform/observability`** : tout serveur gRPC en aura besoin. Chaque niveau
    de grpc sur le même niveau slog, Fatal en Error puis sortie ; verbosité 0. L'Info n'est pas rabattu en Debug : le
    refus de handshake sort en Info (`channelz.Info`, « failed to create ServerTransport », grpc-go 1.84).
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

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche (image construite localement, importée dans k3s sous le tag `proof` ; timer
de déploiement suspendu pendant la preuve), sur le cluster en v0.7.0, OpenBao descellé.

1. `ca.yaml` : `Certificate` `content-key-svc-tls` `Ready`. `networkpolicies.yaml` et le manifest appliqués :
   `rollout status` réussi, pod `1/1`. La sonde `/readyz` du kubelet passe le default-deny alors que la politique du
   service n'ouvre aucune entrée sur le port ops (règle `--src-type LOCAL` de kube-router).
2. Client au SAN `admin-api-svc` (certificat ad hoc, pod au label `admin-api-svc`, sortie temporaire vers 7002) :
   `Seal` rend `vault:v1:…` et `kmsKeyRef: config-secrets` ; l'audit d'OpenBao montre `auth/kubernetes/login` puis
   `transit/encrypt/config-secrets`. `Open` : `PermissionDenied`, « caller not allowed on this method ».
3. Client au SAN `router-svc` depuis un pod admis par la NetworkPolicy : handshake refusé ; le service le journalise
   en JSON (`"logger":"grpc"`, « mtls: client [router-svc] is not an allowed caller [admin-api-svc] »).
4. Pod au label `other` : `connection refused` (entrée refusée par la NetworkPolicy).
5. Non joué sur le VPS : `/readyz` avec OpenBao scellé, qui demande le jeton racine et les parts (geste humain) ;
   le test du `cmd/` le prouve contre un OpenBao injoignable.

Rouge vu : les premiers clients appelaient dès leur démarrage et prenaient « connection refused » avant que
kube-router n'admette leur sortie (`.claude/rules/ci-deploy.md`) ; relancés en `restartPolicy: OnFailure`.

Objets de preuve supprimés ; `content-key-svc` reste déployé sous `proof` jusqu'à la release.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité, tests creux, code en trop). Bloquants : 1 à 4.

1. **L'arrêt n'était pas borné par `SHUTDOWN_TIMEOUT`** : `Stop` attend les handshakes en cours, que grpc-go borne à
   120 s ; une connexion muette retenait le drain au-delà de son budget. Le serveur borne le handshake à
   `SHUTDOWN_TIMEOUT` (`grpc.ConnectionTimeout`) : le listener se ferme au début du drain, aucun handshake ne survit
   donc à l'échéance. Prouvé par une connexion qui n'envoie rien.
2. **La confiance TLS du client OpenBao n'était pas falsifiée** : le client se construit dans une fonction testée
   contre un serveur TLS de la CA (admis), d'une autre CA (refusé), plafonné à TLS 1.2 (refusé).
3. **Le crochet de drain** : le test voit `/readyz` passer à 503 pendant `DRAIN_DELAY`.
4. **`Stop` à l'échéance** : un appel jamais libéré est coupé passé `SHUTDOWN_TIMEOUT`.
5. **Garde resserrée** : un Service sans `type` (ClusterIP) et sélectionnant les pods du `cmd/` ; `rollout status` dans
   la boucle des services ; `TLS_ALLOWED_CLIENTS` présent dans le manifest de `content-key-svc` ; une boucle des
   services sans boucle des Jobs. Écartés : le port de la sonde (un mauvais port fait échouer `rollout status`
   bruyamment) ; le certificat du service (manque hérité de `catalog-apply`).
6. **Tests resserrés** : `GRPC_ADDR` libre après un refus de démarrage, attente bornée de l'appel en vol, sortie de
   `Fatal` en sous-processus.
7. **Retraits** : la colonne `also` de la table de la garde (un remplacement de bloc suffit), `grpcSet` (un port
   valide n'est jamais 0), le commentaire de `checkService`, et dans le manifest les défauts de Kubernetes (`protocol`,
   `replicas`, `targetPort`, `expirationSeconds`, le port nommé `grpc` que rien ne vise, l'`automountServiceAccountToken`
   du ServiceAccount que celui du pod remplace).
8. **Gardés tels quels** : la lecture explicite de la CA (`mtls.Files.Client` chargerait aussi le certificat du
   service) ; trois découpages de listes à virgules dans trois paquets ; le chemin du jeton projeté et l'ordre des
   composants, prouvés sur le VPS et par le superviseur ; l'hypothèse d'`OPENBAO_ADDR` (port et nom du Service égaux à
   ceux des pods), vraie pour la brique, à revoir avec un OpenBao managé (D1).

## Correctifs de revue, tour 2

Constats : 1 bloquant, 8 mineurs.

1. **`targetPort` n'était plus gardé** (bloquant, régression du tour 1) : la garde refuse tout `targetPort` sur le
   Service du `cmd/`, dont le port est déjà celui de `GRPC_ADDR` ; un `targetPort` faux laisserait le Service sans
   endpoint utile alors que `rollout status` passe.
2. **`rollout status` cherché dans la boucle**, ligne exacte, entre l'en-tête des services et son `done` : un
   commentaire, une ligne après `done` ou un `|| true` ne passent plus.
3. **`automountServiceAccountToken: false`** exigé sur chaque pod gardé : seul le jeton projeté d'OpenBao doit être
   monté.
4. **L'échéance de `Stop` mesurée** : l'appel en vol est coupé en moins d'une seconde pour 200 ms ; l'attente de
   l'appel est bornée.
5. **Retraits** : la règle « `TLS_ALLOWED_CLIENTS` présent » (le démarrage et `netpol` refusent déjà son absence) ; le
   saut d'une adresse vide dans le test des refus.
6. **Gardés tels quels** : la borne exacte du handshake n'est prouvée qu'à un facteur près (la connexion muette prouve
   l'arrêt borné, pas la valeur de la borne) ; le câblage du client OpenBao dans `run`, que seul le VPS prouve (le
   chemin du jeton projeté est une constante) ; `SHUTDOWN_TIMEOUT` borne désormais aussi chaque handshake en régime
   normal, à ne pas abaisser sous quelques secondes.

## Correctifs de revue, tour 3

Constats par tour : 21 (4 bloquants), 9 (1 bloquant), 2 ; aucun bloquant au tour 3. La boucle des services doit se
fermer par un `done` seul (un `done || true` neutralise `set -e`) ; le cas « attente après la boucle » garde la
tabulation, pour échouer sur la position et non sur la forme.

## Preuve sur la release v0.8.0

Rejouée le 2026-10-20 sur l'image `content-key-svc:v0.8.0` déployée par le timer (la preuve ci-dessus portait sur
l'image de la branche, avant les correctifs de revue) ; timer suspendu pendant la preuve, puis réarmé.

1. **Appels** : `Seal` depuis le SAN `admin-api-svc` rend `vault:v1:…` et `kmsKeyRef: config-secrets` ; l'audit
   d'OpenBao montre `auth/kubernetes/login` puis `transit/encrypt/config-secrets` : le client OpenBao est bien câblé
   dans `run` (CA interne, jeton projeté). `Open` : `PermissionDenied`. SAN `router-svc` : refusé au handshake, dix
   lignes JSON « mtls: client [router-svc] is not an allowed caller [admin-api-svc] ». Pod au label `other` :
   `connection refused`.
2. **Renouvellement du jeton** : premier `Seal` à 07:38:38 (login, puis `encrypt`), second à 08:11:48, 33 min plus
   tard, passé la moitié du bail d'une heure : un nouveau login précède l'`encrypt`.
3. **Drain d'un vrai pod** (`rollout restart`) : SIGTERM, « draining » à 08:12:52, pod disparu avant 08:12:59
   (`DRAIN_DELAY` de 5 s, puis arrêt) ; ni « drain budget exceeded » ni « content-key-svc failed ».

4. **OpenBao scellé** (scellement et descellement par l'humain, le 2026-10-20) : avant, `/readyz` 200, pod prêt,
   `Seal` admis. Scellé à 08:36 : `/readyz` reste 200, le pod reste prêt (ni rollout ni timer bloqué) ; `Seal` rend
   `Unavailable`, « KMS unavailable », sans détail d'OpenBao. Descellé à 08:39 : `Seal` réussit sans nouveau login,
   le jeton survit au scellement.

Constat : scellé après un descellement, OpenBao sert deux séries, `vault_core_unsealed{cluster="vault-cluster-…"} 0`
et un résidu `{cluster=""} 1` de son démarrage scellé, quand il ne connaissait pas encore le nom de son cluster.
`OpenBaoSealed` (`vault_core_unsealed == 0 or absent(…)`) part quand même, puisqu'elle retient toute série à 0 ; une
règle qui agrégerait les séries par `min` ou `max` devrait en tenir compte.
