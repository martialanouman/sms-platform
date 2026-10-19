# Step 007b-1 — Livraison : `catalog-apply`, topics et ACL Kafka

Plan : `docs/plan/socle.md` §007, deuxième des six PR. Spec : `docs/spec/09-contrats.md` §9.1 (garde 7),
`docs/spec/02-architecture.md` §2.2 (Jobs), `docs/spec/10-exploitation.md` §10.2. Décision :
`docs/design/06-confiance-interne.md` §5.2. Règles : `.claude/rules/topics.md`, `.claude/rules/ci-deploy.md`,
`.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

`contracts/topics.yaml` déclare 20 topics et 38 groupes ; `internal/catalog` le lit, le valide et dérive
les dead-letters (004a). Rien ne l'applique : go-gateway créait ses topics par `kafka-provision` (partitions
seules, sans rétention ni ACL) et n'avait aucune ACL. Le dépôt n'a aucun client Kafka.

La spec : « un Job idempotent en applique partitions, rétention, `cleanup.policy` et ACL » (09 §9.1) ;
« les ACL sont générées depuis le catalogue (`producers`, groupes) et appliquées par le même Job. Aucune
politique écrite à la main » ; le principal Kafka est le CN du certificat (06 §5.2) ; garde 7 : « les ACL
Kafka générées correspondent exactement au catalogue » ; `check-generated` couvre les ACL (C3). Ordre de
déploiement : `migrate`, puis `catalog-apply`, puis les services (10 §10.2).

**Terminé quand** (part de 007b-1) : `catalog-apply` crée sur un Kafka réel chaque topic du catalogue et
chaque dead-letter, avec ses partitions, sa rétention et sa `cleanup.policy`, et laisse exactement les ACL
dérivées du catalogue ; une seconde exécution ne change rien.

## Arbitrages

Fable a tranché les propositions le 2026-10-19 (P1, P3 à P7, P9, P10 tenues ; P2 et P8 amendées) :

- **P1 — Mécanisme** : l'API d'administration Kafka (franz-go `kadm`), pas les CRD Strimzi : MSK et
  Confluent, substituts écrits (02 §2.6), n'en ont pas, et la spec nomme un Job. Pour 007d-1 : Strimzi
  sans Topic Operator ni User Operator (deux écrivains sinon), `super.users=User:CN=catalog-apply`.
- **P2 — Topics** (amendée) : un absent est créé (partitions, `cleanup.policy` `delete` ou `compact`,
  `retention.ms` hors compacté), facteur de réplication et `min.insync.replicas` aux défauts du broker
  (spec 10 §10.3 : réglages de cluster, posés en 007d-1). Un existant voit ces deux configs alignées par
  `IncrementalAlterConfigs`, comparées avant écriture : ce que pose l'exploitant reste intact. Partitions :
  `per_key`, le cluster égale le catalogue, sinon refus ; ordre `per_partition` (clé `manual`, `mt.routed.<cat>`), le catalogue
  est un **plancher**, l'Admin API étend ces topics par blocs (design 03 §4.2, spec 07 §7.3) : moins est
  étendu, plus est l'état normal ; `none` et dead-letters, le catalogue fait foi : moins est étendu, plus
  est refusé. Un topic hors catalogue n'est jamais supprimé : il est nommé dans le journal (`ListTopics`
  exclut déjà les internes).
- **P3 — ACL dérivées** : principal `User:CN=<service>`, ressources `LITERAL`, `ALLOW`, hôte `*`. Un
  producteur : `WRITE` sur le topic (qui implique `DESCRIBE` et, depuis KIP-679, `IDEMPOTENT_WRITE`). Un
  groupe : `READ` sur son topic, `READ` sur le groupe `<groupe>`, `WRITE` sur sa dead-letter. Une règle
  unique : `manual` commite dans son groupe hors adhésion (design 03 §4.3) ; `broadcast` n'a pas de
  contrat de commit écrit, le runtime de lane amendera la dérivation dans sa PR s'il fait autrement. Le
  rejeu (`replay-dead-letter`, dans `admin-api-svc`, design 05 §5) heurte la garde 2 : sa règle se tranche
  avec lui. De même l'`ALTER` qu'exige l'extension de `mt.routed.<cat>` par l'Admin API (design 03 §4.2) :
  ni l'opération ni son principal n'existent, son droit entre avec elle et « exactement » le retirerait
  s'il était posé à la main. Séquencement, pas une dette.
- **P4 — Exactement** : l'ensemble des ACL du cluster devient l'ensemble dérivé, quel qu'en soit le
  principal (« aucune politique écrite à la main », 06 §5.2) ; le super-utilisateur n'en a pas besoin.
  Les manquantes sont créées **avant** que les excédentaires soient retirées : un droit qui demeure n'est
  jamais absent.
- **P5 — Générées et relues** : `docs/topics.md` (déjà sous `check-generated`) gagne une table des ACL,
  une ligne par droit, rendue par la fonction même que le Job applique. Un fichier distinct ne serait
  consommé par rien.
- **P6 — Cœur et adaptateur** : `internal/catalog` dérive les ACL et calcule le plan (pur, refus
  compris) ; `internal/catalog/kafka` lit le cluster et applique le plan par `kadm` ; `cmd/catalog-apply`
  compose. Le catalogue est embarqué (`contracts`, `//go:embed`), comme les migrations.
- **P7 — Connexion** : `KAFKA_BROKERS` (liste, virgules) entre dans `config.Infra`, sans défaut ;
  `catalog-apply` refuse des brokers vides et exige mTLS (`mtls.Files.Client`, franz-go pose le
  `ServerName` par broker) : jamais de repli en clair.
- **P8 — Concurrence** (amendée) : ni code ni test. La spec dit « idempotent » ; un Job à nom fixe est
  seul dans son namespace et 007d-2 déploie en séquence ; une course ferait échouer le Job
  (`backoffLimit: 0`) et le déploiement suivant convergerait.
- **P9 — Manifest** : `deploy/k8s/catalog-apply.yaml`, un Job comme `migrate`, `KAFKA_BROKERS` en
  valeur, `TLS_*` vers le volume du Secret `catalog-apply-tls` (cert-manager, 007d-1). Aucune règle de
  garde neuve. Borne de temps et tag en 007d-2.
- **P10 — Tests** : Kafka réel par testcontainers (module `kafka`, KRaft), `StandardAuthorizer` et
  `User:ANONYMOUS` en super-utilisateur, sur l'adaptateur (le mTLS de `cmd` ne se dresse pas contre un
  conteneur en clair) ; ce qu'une ACL interdit relève de Kafka, la step prouve l'ensemble posé.

## Design arrêté

- `contracts/contracts.go` : `Topics() []byte`, `topics.yaml` embarqué.
- `internal/catalog` :
  - `ACL{Principal, Resource, Name, Pattern, Operation, Permission, Host}` ; `(Catalog) ACLs() []ACL`,
    triées, sans doublon ;
  - `(Topic) Configs() map[string]string` : `cleanup.policy`, et `retention.ms` hors compacté ;
  - `State{Topics map[string]Live, ACLs []ACL}`, `Live{Partitions, Configs}` ; `(Catalog) Plan(State)
    (Plan, error)` sur les topics du catalogue et les dead-letters : créations, extensions, configs à
    écrire, ACL à ajouter puis à retirer, topics hors catalogue ; tous les refus d'un coup, aucun plan
    s'il en est un ; `(Plan) Empty() bool` ;
  - `Markdown` : partitions d'un topic à clé `manual` rendues « 384 (initial) », table des ACL.
- `internal/catalog/kafka` : `Sync(ctx, *kgo.Client, Catalog) (Plan, error)` lit le cluster par `kadm`,
  calcule le plan, n'écrit rien s'il est refusé ; sinon crée, étend, configure, ajoute puis retire les ACL,
  une ACL exacte par entrée (`kmsg` : le constructeur de `kadm` fait un produit cartésien).
- `Topic.Partitions` en `int32`, le type du protocole Kafka.
- `internal/platform/config` : `KAFKA_BROKERS` → `Infra.KafkaBrokers`, sans défaut.
- `cmd/catalog-apply` : `run(ctx, config.Infra, *slog.Logger) error` refuse des brokers vides et des
  fichiers TLS absents, se connecte en mTLS, appelle `Sync`, journalise chaque changement et les topics
  hors catalogue ; `main` comme `migrate`.
- `internal/platform/mtls/mtlstest` : l'autorité de test des tests mTLS, sortie pour que `catalog-apply`
  prouve son dial TLS.
- GoReleaser, `deploy/k8s/catalog-apply.yaml`, la garde de livraison vérifie le nouveau couple.
- Tests : unitaires du plan (chaque règle de partitions, configs, ACL en trop et manquantes, hors
  catalogue, refus groupés) et de la dérivation ; intégration de `Sync` (cluster vide → topics, configs et
  ACL égaux au catalogue et à `ACLs()`, une ACL étrangère retirée ; seconde passe : plan vide ; un refus
  n'écrit rien) ; `run` refuse brokers vides et TLS absent, sans connexion, et son premier octet vers un
  broker ouvre une poignée de main TLS. Kafka KRaft rend topics, configs et ACL visibles après coup : les
  tests attendent l'état, bornés à 30 s ; après un refus, un marqueur écrit puis vu prouve, par l'ordre du
  journal de métadonnées, que rien d'antérieur n'a été écrit.

## Revue

**Tour 1** (trois axes, aucun bloquant) :

- Retenus : l'ACL étrangère du test portait sur une ressource sans droit dérivé (un filtre de retrait
  sans principal survivait) ; une dead-letter de source `manual` héritait du plancher (le plancher suit
  l'ordre `per_partition`, plus la clé) ; erreurs de retrait par ACL ignorées ; lecture des ACL réécrite
  en `kmsg` là où `kadm` suffit ; « (initial) » et les attributs du journal non vérifiés ; doc de `Plan`
  et `ACLs` qui redit le code ; `Sync`, `mtlstest` et le test du premier octet absents du design.
- Écartés : `Validate` dans `catalog-apply` (le fichier embarqué est celui que la CI valide, aucun test ne
  ferait tomber l'appel) ; une preuve de l'ordre « ajout avant retrait » exigerait un refus de Kafka sur une
  ACL bien formée ; `Authority.PEM` reste exporté (`mtls_test.go` lit les PEM) ; `Live.Partitions` en
  `int32` déplacerait la conversion vers `len`, que gosec refuse.
- Pour 007d-1 : le principal est `User:CN=<service>` seulement si le sujet du certificat se réduit au CN
  (ou par `ssl.principal.mapping.rules`), `super.users=User:CN=catalog-apply` compris.
