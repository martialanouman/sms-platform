# Step 007d-1 — Livraison : briques de l'environnement de test

Plan : `docs/plan/socle.md` §007, cinquième des six PR. Spec : `docs/spec/02-architecture.md` §2.5 (une CA
interne, mTLS, identités par magasin) et §2.6 (briques autohébergées), `docs/spec/10-exploitation.md` §10.2-10.3.
Décisions : `docs/design/06-confiance-interne.md` (§4, §5, §8). Règles : `.claude/rules/ci-deploy.md`,
`.claude/rules/tests.md`. Mode : PR.

## Contexte

Les Jobs `migrate` et `catalog-apply` et leurs NetworkPolicy existent (007a à 007c) ; rien ne fournit ce
qu'ils supposent. Ce que les steps précédentes ont renvoyé ici :

- **Postgres** (CloudNativePG) : cluster `postgres` en namespace `postgres`, pods `cnpg.io/cluster:
  postgres`, 5432 (007c) ; base `control_plane` possédée par `migrator`, `LOGIN CREATEROLE` (006a, 007b-2) ;
  TLS vers Postgres (007a) ; réplication 3 et `min.insync.replicas` sont des réglages de cluster (spec 10 §10.3,
  007b-1) ; `breakglass` et le rôle `CREATEROLE` d'OpenBao créés sous un autre rôle que `migrator` (007b-2).
- **Kafka** (Strimzi) : cluster `kafka` en namespace `kafka`, bootstrap `kafka-kafka-bootstrap.kafka.svc:9093`
  (007c) ; listener mTLS, principal `User:CN=<service>` (le sujet du certificat se réduit au CN),
  `super.users=User:CN=catalog-apply`, ni Topic ni User Operator (007b-1).
- **cert-manager** : une CA interne qui signe tout (06 §4) ; le Secret `catalog-apply-tls` (007b-1).
- **Secrets** : `postgres-password` de chaque service déclaré (32 caractères ASCII imprimables au moins,
  tirés au hasard) et `catalog-apply` (URL sous `migrator`), avant tout service ; `migrate` (URL) (007a).
  Le dépôt est public : aucun secret n'y entre.
- **k3s** applique les NetworkPolicy par son contrôleur intégré, à ne pas désactiver (007c).

Versions retenues : k3s v1.37.1+k3s1 ; cert-manager v1.21.2 ; CloudNativePG 1.30.1 (chart `cloudnative-pg`
0.29.1), image `ghcr.io/cloudnative-pg/postgresql:18.6-system-trixie` épinglée par digest ; Strimzi 1.3.0,
Kafka 4.3.1 (métadonnées `4.3-IV0`).

VPS de test : `contabo169` (Rocky Linux 10.2, 8 vCPU, 23 Go), dédié, réinitialisable (décision humaine du
2026-10-19). Le dépôt ne nomme que l'alias SSH : ni adresse, ni kubeconfig, ni sortie non expurgée.

**Terminé quand** (part de 007d-1) : sur un k3s, les briques s'installent à version épinglée ; `migrate`
puis `catalog-apply` y passent sous TLS, mTLS et ACL réels ; aucun secret n'est dans le dépôt.

## Arbitrages

Fable a tranché le 2026-10-19 (B1, B4, B5 tenues et précisées ; B2, B3 amendées ; B6 et B7 tranchées avec le
VPS) :

- **B1 — Installation** : le `HelmChart` intégré de k3s, chart et version épinglés, un fichier par brique sous
  `deploy/bricks/`. La portabilité (02 §2.6) porte sur les briques, pas sur leur installeur : sur un cloud,
  Postgres et Kafka sont des services managés et cert-manager se pose par l'outillage du fournisseur ; 10
  §10.2 fixe l'environnement de test à k3s. Ni copie des manifests amont, ni téléchargement à somme figée
  (`HelmChart` n'en a pas ; Helm vérifie le digest de l'index ou de l'OCI). Les CR ne s'appliquent qu'après
  les Jobs `helm-install-*` : l'ordre vit dans la procédure, puis dans le déploiement de 007d-2. Le
  helm-controller de k3s ne se désactive pas.
- **B2 — Postgres** (amendée) : une instance par nœud, sans `synchronous` : sur une VM à un nœud, trois
  instances partagent hôte et disque, et 10 §10.3 protège `control_plane` par PITR, pas par réplication
  synchrone. Le nombre d'instances est une valeur d'environnement, comme la mémoire des JVM. `bootstrap.initdb` (`control_plane`,
  propriétaire `migrator`) et `managed.roles` (`migrator`, `LOGIN CREATEROLE`, même Secret). `openbao` et
  `breakglass` entrent avec la brique OpenBao, que rien n'exerce avant elle (un rôle géré sans Secret naît à
  mot de passe NULL) ; `managed.roles` tiendra alors « créés sous un autre rôle que `migrator` ». Séquencement,
  plan amendé.
- **B3 — Une racine** (amendée) : un `Certificate` racine cert-manager (`isCA`, `duration: 87600h`,
  `privateKey.rotationPolicy: Never` : une racine à clé tournante casserait toute feuille) derrière un
  `ClusterIssuer` `internal-ca`. Elle émet le certificat serveur de Postgres (`serverTLSSecret` et
  `serverCASecret` seuls ; rien de nôtre ne s'authentifie à Postgres par certificat) et celui du listener
  9093 (`brokerCertChainAndKey`, SAN du bootstrap et `*.kafka-kafka-brokers.kafka.svc`, clé PKCS#8, que
  Strimzi exige) ; elle est la CA de clients de Strimzi (`clientsCa.generateCertificateAuthority: false`,
  Secrets `kafka-clients-ca-cert` et `kafka-clients-ca` copiés par `secrets.sh`) et signe `catalog-apply-tls`
  (CN `catalog-apply` seul, `client auth`). La CA de cluster reste à Strimzi : nos clients ne la voient
  jamais, et la clé racine ne signe pas ses composants internes. `mtls.Files.CA` à un fichier suffit. URL
  `sslmode=verify-full&sslrootcert=/tls/ca.crt` ; `migrate` monte la racine depuis un ConfigMap `internal-ca`.
- **B4 — Kafka** : un `KafkaNodePool` `kafka` (trois nœuds `controller` et `broker`, `persistent-claim` sur le
  `local-path` de k3s, `resources` et `-Xmx` bornés sur la mesure du VPS) ; `spec.kafka.version` et
  `metadataVersion` explicites ; `default.replication.factor` 3, `min.insync.replicas` 2, journaux internes
  à 3 et 2, `auto.create.topics.enable: false` ; listener `tls` 9093, authentification `tls`, autorisation
  `simple`, `superUsers: [CN=catalog-apply]`, sans `entityOperator`.
- **B5 — Secrets** : `deploy/bricks/secrets.sh`, shell (kubectl vient avec k3s, `openssl` est sur la VM ; un
  outil Go exécuterait kubectl ou tirerait client-go, écarté en 007b-2). `set -euo pipefail`, jamais `set -x`,
  aucune valeur affichée ; `openssl rand -hex 32` ; un Secret présent n'est jamais recréé (`get || create`) ;
  le mot de passe de `migrator` se relit du Secret existant et les URL en dérivent.
- **B6 — Preuve** (décision humaine, recommandation de Fable) : une procédure écrite ici, rejouée à la main
  sur `contabo169` avant la PR, de la table rase à la table rase, ses rouges compris ; pas de k3s
  testcontainers en CI (6 à 10 min et ~1 Go par passage). Après 007d-2, chaque merge se prouve sur la même VM ;
  la procédure est la spécification du bootstrap de 007d-2.
- **B7 — Une PR**. Ce qui manquait : le PITR de 10 §10.3 exige un stockage objet (plugin Barman Cloud de
  CloudNativePG) que RustFS apportera : **report, issue `debt` et `go-live`** ; les politiques des namespaces
  des briques (Kafka : `networkPolicyPeers` sur le listener, Strimzi génère la politique ; Postgres : une
  politique d'entrée écrite avec la brique), configuration d'une brique et non génération ; l'alerte
  d'échéance des certificats, dont la métrique naît avec cert-manager.

Arbitré pendant la procédure (Fable, 2026-10-19), sur un fait du VPS : le contrôleur de politiques de k3s
(kube-router) n'admet l'adresse d'un pod neuf qu'après quelques secondes ; jusque-là sa sortie tombe sous
`default-deny` (REJECT). `migrate` et `catalog-apply`, qui se connectent dès leur démarrage, échouaient à
chaque passage ; précédés de dix secondes d'attente, ils passent. Un pod relancé a une adresse neuve et la
même course. Décision : **retentative dans le code, pour Postgres seul** ; franz-go tient déjà les erreurs de
dial pour transitoires (`isPermanentDialErr`) et les rejoue jusqu'à `RetryTimeout` (30 s). Un helper
`internal/platform/pgdial`, `Retry(ctx, attempt)`, rejoue toutes les secondes tant que l'erreur se déplie en
`net.Error`, et rend toute autre erreur (authentification, `verify-full`) au premier essai ; chaque `cmd/` le
borne à 60 s : il couvre la course du CNI, pas l'attente d'une brique, que le déploiement fait par
`kubectl wait`. Rejetés : un `initContainer` d'attente (une image de plus, un délai deviné), une politique
relâchée (02 §2.5).

Second fait du VPS (Fable, 2026-10-19) : sur trois brokers, un `catalog-apply` relancé dans les secondes qui
suivent le premier échoue dans `read` (« topic mo.routed: UNKNOWN_TOPIC_OR_PARTITION ») : `ListTopics` et
`DescribeTopicConfigs` sont servis par des brokers dont la métadonnée n'a pas encore convergé ; une minute plus
tard, « cluster already matches the catalog ». Aucune lecture périmée ne produit d'écriture nuisible (au pire
`TOPIC_ALREADY_EXISTS` ou `INVALID_PARTITIONS`, bruyants). Décision : **`catalog-apply` passe à
`backoffLimit: 2`**, Kubernetes rejoue le Job entier, dont l'idempotence rend le rejeu sûr ; `migrate` reste à
0. Rejeté : une retentative de `read` sur un seul code d'erreur, qui ne couvre qu'une issue sur quatre. Plafond :
un refus déterministe du plan tourne trois fois ; `podFailurePolicy` l'en distinguera si cela coûte un jour.

Arbitrage de la revue (Fable, 2026-10-19) :

- **Qui demande un certificat** : `global.rbac.aggregateClusterRoles: false` dans le chart de cert-manager ;
  plus aucun `edit` ni `admin` ne porte `certificates`. Aujourd'hui seul l'administrateur de la VM applique :
  rien à différer. Approver-policy entre avec le premier compte qui reçoit `edit` (règle `ci-deploy.md`).
- **La clé racine ne quitte jamais `cert-manager`** (B3 amendée) : une CA intermédiaire ne bornerait rien,
  tout vérificateur qui fait confiance à la racine accepterait un certificat forgé sous elle. Le listener
  9093 passe en authentification `custom` (`ssl.client.auth: required`, magasin de confiance PEM monté depuis
  le Secret `kafka/internal-ca-cert`, certificat seul) ; le principal reste `User:CN=catalog-apply`
  (`DefaultKafkaPrincipalBuilder`). Plus de `clientsCa` fournie ; 06 §4 amendée.
- **`pgdial.Retry`** ne rejoue qu'un `*net.OpError` de `dial` (refus, délai, nom non résolu, comme
  `isAnyDialErr` de franz-go) ; une alerte TLS du serveur, une lecture ou une écriture coupée sortent au
  premier essai. La borne d'une minute est interne à `Retry`.
- **Images** : la version du chart épingle le tag de release, que les registres des projets ne réécrivent pas ;
  `spec.kafka.version` fixe l'image Kafka. Le digest reste à Postgres, dont le tag majeur couvre plusieurs builds.
- **Alertes et rotation** : `CertificateNotReady` (`certmanager_certificate_ready_status{condition="False"}`,
  `for: 10m`) couvre un certificat jamais émis ; les copies du certificat racine (`kafka/internal-ca-cert`,
  `sms/internal-ca`) passent par `apply` à chaque `secrets.sh`, donc à chaque déploiement. Reste au runbook :
  un rolling update de Kafka pour relire son magasin de confiance après un renouvellement de la racine.

## Design arrêté

- Namespace de la plateforme : `sms`.
- `deploy/bricks/` :
  - `operators.yaml` : trois `HelmChart` (`kube-system`) : cert-manager `oci://quay.io/jetstack/charts/cert-manager`
    v1.21.2 (`crds.enabled`, namespace `cert-manager`) ; CloudNativePG `cloudnative-pg` 0.29.1 du dépôt
    `https://cloudnative-pg.github.io/charts` (namespace `cnpg-system`) ; Strimzi
    `oci://quay.io/strimzi-helm/strimzi-kafka-operator` 1.3.0 (namespace `kafka`, qu'il surveille) ;
  - `ca.yaml` : `ClusterIssuer` autosigné, `Certificate` racine `internal-ca` (`cert-manager`), `ClusterIssuer`
    `internal-ca` ; feuilles `postgres-server` (`postgres`), `kafka-listener` (`kafka`), `catalog-apply-tls`
    (`sms`, CN seul, `client auth`) ;
  - `namespaces.yaml` : `sms` et `postgres` ;
  - `postgres.yaml` : `Cluster` `postgres` (image par digest, une instance,
    `initdb`, `managed.roles`, certificats serveur), `NetworkPolicy` d'entrée : 5432 depuis `sms`, 5432 depuis
    le cluster, 8000 depuis l'opérateur ;
  - `kafka.yaml` : `KafkaNodePool` et `Kafka` `kafka` (B4), listener en authentification `custom` contre la
    racine montée, `networkPolicyPeers` vers `sms` ;
  - `secrets.sh` (valeurs sans saut de ligne final) : `postgres/migrator` (basic-auth), `sms/migrate` et `sms/catalog-apply` (`postgres-url`),
    `sms/<service>` (`postgres-password`) par fichier de `contracts/grants/` ; chaque valeur vérifiée non vide
    et passée par l'entrée standard ; le certificat racine seul, par `apply`, dans `kafka/internal-ca-cert`
    et le ConfigMap `sms/internal-ca`.
- `deploy/k8s/migrate.yaml` : la racine montée en `/tls` depuis le ConfigMap `internal-ca`.
- `deploy/k8s/catalog-apply.yaml` : `backoffLimit: 2`.
- `internal/platform/pgdial` : `Retry(ctx, *slog.Logger, func(context.Context) error) error`, borné à une
  minute ; chaque retentative journalisée sans URL. `catalog-apply` enveloppe `pgx.Connect`, `migrate` un
  `db.PingContext` avant goose. Tests : un port fermé et un nom non résolu rejoués jusqu'au succès, une
  `PgError`, une erreur x509 et une alerte TLS distante rendues au premier essai, un contexte expiré rend la
  dernière erreur de dial ; dans chaque `cmd/`, une base derrière `tcpproxy.Later` (refus pendant deux
  secondes, puis relais) atteinte.
- `.claude/rules/ci-deploy.md` : la course de k3s et le `/readyz` ; la métadonnée Kafka et le rejeu entier ;
  qui demande un certificat ; l'épinglage des images de brique.
- `deploy/guard_test.go` : `deploy/bricks/` nomme ce que supposent `internal/netpol` et `catalog-apply` :
  `Cluster` `postgres` en `postgres`, `Kafka` `kafka` en `kafka`, listener 9093 TLS à certificat client
  exigé, autorisation `simple` et `superUsers` `CN=catalog-apply`, ni `entityOperator` ni `clientsCa` ; un cas
  faussé par règle.
- `deploy/alerts/certs.yaml` (`CertificateExpiresSoon`, échéance à moins de 7 jours ; `CertificateNotReady`)
  et `certs_test.yaml`.
- Plan §007d-1 : `openbao` et `breakglass` avec la brique OpenBao. Issue `debt` + `go-live` : PITR.
- Procédure (section suivante), jouée sur `contabo169`, résultats consignés avant la PR.

## Preuve sur `contabo169`

Procédure jouée le 2026-10-19 au commit `3df8fc9`, de la table rase (`k3s-uninstall.sh`) à la table rase ; images
du snapshot importées dans le containerd de k3s (`docker save | k3s ctr images import`). Sorties expurgées des
adresses.

1. k3s `v1.37.1+k3s1`, SELinux appliqué (`k3s-selinux`) ; `operators.yaml`, puis `kubectl wait` des Jobs
   `helm-install-cert-manager`, `-cloudnative-pg`, `-strimzi` : `complete`.
2. `namespaces.yaml`, `ca.yaml` : les quatre `Certificate` `Ready`.
3. `secrets.sh` deux fois : `resourceVersion` des six Secrets inchangés ; `postgres-password` de 64 caractères.
4. `postgres.yaml`, `kafka.yaml` : `Cluster` sain, `Kafka` `Ready` en 4.3.1 / `4.3-IV0`, trois nœuds.
5. `networkpolicies.yaml` puis les Jobs, sans attente artificielle :
   - `migrate` : `succeeded 1`, « postgres unreachable, retrying » puis « migration applied », version 1 ;
   - `catalog-apply` : `succeeded 1`, 58 topics et 139 ACL posés, le secret de `admin_api_svc` posé ;
   - un second `catalog-apply` aussitôt : `succeeded 1, failed 1`, la tentative rejetée sur « topic
     mt.billing-wait.transactional: UNKNOWN_TOPIC_OR_PARTITION », rejouée ; relancé plus tard, « cluster
     already matches the catalog » (deux passages).

Rouges vus :

- **Course du contrôleur de politiques** (avant `pgdial`) : les deux Jobs en `Failed` à chaque passage, « dial
  tcp …:5432: connect: connection refused » ; avec dix secondes d'attente, `migrate` passe.
- **Secret à saut de ligne** (avant le correctif de `secrets.sh`) : `catalog-apply` en `Failed`, « role
  admin_api_svc: secret outside printable ASCII » (le refus de 007b-2).
- **Métadonnée pas encore propagée** (avant `backoffLimit: 2`) : le second passage en `Failed`, « topic mo.routed:
  UNKNOWN_TOPIC_OR_PARTITION ».
- **Certificat de la même CA, CN `intruder`** : `failed 3`, « describe ACLs: CLUSTER_AUTHORIZATION_FAILED ».
- **Certificat d'une autre CA** : `failed 3`, « list topics: remote error: tls: unknown certificate ».
- **`migrate` contre une CA étrangère** : `failed 1`, `verify-full` refuse le certificat de Postgres.
- **Flux non générés** (busybox, après huit secondes) : `migrate` → Kafka 9093 et un pod sans label → Postgres
  5432 sortent en `exit=1` ; `migrate` → Postgres et `catalog-apply` → Kafka : `open`.
