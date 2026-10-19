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
- **B2 — Postgres** (amendée) : `instances: 1`, sans `synchronous` : sur une VM à un nœud, trois instances
  partagent hôte et disque, et 10 §10.3 protège `control_plane` par PITR, pas par réplication synchrone ;
  trois instances avec le premier environnement à plusieurs nœuds. `bootstrap.initdb` (`control_plane`,
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

## Design arrêté

- Namespace de la plateforme : `sms`.
- `deploy/bricks/` :
  - `operators.yaml` : trois `HelmChart` (`kube-system`) : cert-manager `oci://quay.io/jetstack/charts/cert-manager`
    v1.21.2 (`crds.enabled`, namespace `cert-manager`) ; CloudNativePG `cloudnative-pg` 0.29.1 du dépôt
    `https://cloudnative-pg.github.io/charts` (namespace `cnpg-system`) ; Strimzi
    `oci://quay.io/strimzi-helm/strimzi-kafka-operator` 1.3.0 (namespace `kafka`, qu'il surveille) ;
  - `ca.yaml` : `ClusterIssuer` autosigné, `Certificate` racine `internal-ca` (`cert-manager`), `ClusterIssuer`
    `internal-ca` ; feuilles `postgres-server` (`postgres`), `kafka-listener` (`kafka`), `catalog-apply-tls`
    (`sms`) ;
  - `postgres.yaml` : `Namespace` `postgres`, `Cluster` `postgres` (image par digest, une instance,
    `initdb`, `managed.roles`, certificats serveur), `NetworkPolicy` d'entrée : 5432 depuis `sms`, 5432 depuis
    le cluster, 8000 depuis l'opérateur ;
  - `kafka.yaml` : `KafkaNodePool` et `Kafka` `kafka` (B4), `networkPolicyPeers` vers `sms` ;
  - `secrets.sh` : `postgres/migrator` (basic-auth), `sms/migrate` et `sms/catalog-apply` (`postgres-url`),
    `sms/<service>` (`postgres-password`) par fichier de `contracts/grants/`, `kafka/kafka-clients-ca-cert` et
    `kafka/kafka-clients-ca` depuis la racine, ConfigMap `sms/internal-ca`.
- `deploy/k8s/migrate.yaml` : la racine montée en `/tls` depuis le ConfigMap `internal-ca`.
- `deploy/guard_test.go` : `deploy/bricks/` nomme ce que supposent `internal/netpol` et `catalog-apply` :
  `Cluster` `postgres` en `postgres`, `Kafka` `kafka` en `kafka`, listener 9093 `tls`, `superUsers`
  `CN=catalog-apply`, sans `entityOperator` ; cas faussés.
- `deploy/alerts/certs.yaml` (`CertificateExpiresSoon`, échéance à moins de 7 jours) et `certs_test.yaml`.
- Plan §007d-1 : `openbao` et `breakglass` avec la brique OpenBao. Issue `debt` + `go-live` : PITR.
- Procédure (section suivante), jouée sur `contabo169`, résultats consignés avant la PR.
