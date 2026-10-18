# Step 005d — Bibliothèques : configuration d'infrastructure, drain

Plan : `docs/plan/socle.md` §005, quatrième des cinq PR. Spec : `docs/design/07-distribution-de-config-et-redis.md`
§3.4 (garde 3), `docs/design/03-connecteur-comptes-baux.md` (D6), `docs/spec/10-exploitation.md` §10.2,
`docs/design/06-confiance-interne.md` §4, `docs/design/00-audit-architecture.md` T4. Mode : PR.

## Contexte

go-gateway lisait dans une seule structure `Config` de 1 469 lignes l'infrastructure et le métier, par
sections déclarées binaire par binaire et gardées par un test de 185 lignes ; il avait deux superviseurs
(`Group`, `Ordered`) et un serveur ops qui portait lui-même le délai du drain. Aucun `cmd/` n'existe ici.

**Terminé quand** (part de 005d) : le chargeur, le superviseur et le serveur ops ont leurs tests ; la garde
de l'environnement, l'ordre du drain et la bascule de `/readyz` ont été prouvés par une mutation.

## Arbitrages

Fable a tranché le 2026-10-18 ; l'humain, le port ops :

- **Chargeur** : une structure plate, `config.Infra`, lue par `config.Load()` ; la structure est la liste
  fermée des variables. Écrit à la main (`os.Getenv`, `time.ParseDuration`, `strconv`), sans
  dépendance ; une variable posée vide vaut son défaut. Seuls les champs qui ont un consommateur : `LOG_LEVEL` (`info`), `OPS_ADDR` (`:9090`),
  `DRAIN_DELAY` (5 s), `DRAIN_BUDGET` (90 s), `SHUTDOWN_TIMEOUT` (30 s), `TLS_CERT_FILE`, `TLS_KEY_FILE`,
  `TLS_CA_FILE`, `TLS_ALLOWED_CLIENTS` (liste séparée par des virgules), `TRACE_SAMPLE_RATIO` (1, la queue
  est au collecteur). Ni `Environment` (aucun usage depuis la 005c), ni endpoint OTLP (dette #93, au premier
  `cmd/`), ni nom de service (une constante du binaire).
- **Garde** : forbidigo refuse `os.Getenv`, `os.LookupEnv`, `os.Environ`, `os.ExpandEnv`,
  `syscall.Getenv`, `syscall.Environ`, `unix.Getenv`, `unix.Environ` et `exec.Cmd.Environ` hors de
  `internal/platform/config` ; les deux
  outils de CI qui lisent l'environnement de la CI (`tools/catalog`, `tools/debtguard`) sont exclus par
  nom, pour qu'un Job déployé composé sous `tools/` reste gardé. Lire `/proc/self/environ` relève de la
  revue, comme les bibliothèques qui lisent l'environnement d'elles-mêmes (pgx et ses `PG*`, le SDK OTel
  et ses `OTEL_*`) : chacune reçoit une configuration explicite. Distinguer `DRAIN_DELAY` de `MAX_BINDS` est un
  jugement de revue, pas d'analyseur : un champ ajouté à la structure passe en revue.
- **Validation** : `DRAIN_DELAY` ≥ 0, `SHUTDOWN_TIMEOUT` > 0, `DRAIN_BUDGET` > `SHUTDOWN_TIMEOUT` (un
  composant a droit à tout son délai, et le drain est séquentiel), ratio dans [0, 1] (`NaN` refusé),
  toute valeur illisible refusée ; une durée illisible n'est confrontée à aucune règle, ni la sienne ni
  celle d'une autre variable : une seule erreur par variable ; les erreurs sont jointes pour qu'un démarrage les montre toutes. Rien sur les
  fichiers TLS ni l'allowlist, que `mtls` refuse déjà à la construction ; rien sur `OPS_ADDR`, que
  `net.Listen` refuse mieux.
- **Superviseur** : `supervisor.Ordered{DrainDelay, DrainBudget}` seul. `Run` : à l'arrêt demandé ou au
  premier retour d'un composant (une erreur, ou un `nil` prématuré, rendu `"<nom>: stopped before
  drain"`), les rappels `OnDrain`, puis `DRAIN_DELAY`, puis l'arrêt des composants **dans l'ordre inverse
  de leur ajout**, un à la fois, sous `DRAIN_BUDGET` ; au-delà, le composant attendu est nommé et les
  suivants sont annulés sans attente. `Run` rend la première erreur, sinon `ErrDrainBudgetExceeded`,
  sinon nil. Un composant qui rend `context.Canceled` à l'arrêt s'est arrêté proprement. Le budget
  est global, il court après `DRAIN_DELAY`. Un rappel `OnDrain` est instantané (une bascule) : il n'est
  pas borné. Amendé : spec
  10 §10.2 (« dans l'ordre inverse de leur enregistrement »). Les gardes de
  go-gateway sur les `cmd/` viennent avec le premier `cmd/` : elles ne garderaient rien ici.
- **Serveur ops** (`internal/platform/ops`) : HTTP en clair sur `OPS_ADDR`, hors mTLS, borné par la
  NetworkPolicy au kubelet et à Prometheus (décision humaine ; amendé : décision 6 §4, spec 02 §2.5).
  `/healthz` (200 tant que le processus vit), `/readyz` (503 `draining` dès le drain, sans sonder ; sinon
  les sondes vitales en parallèle, et 503 avec le nom de chaque sonde en échec ou sans réponse au bout de
  2 s, même si elle ignore son contexte ; une sonde encore en vol d'une requête précédente compte en échec
  sans être relancée, pour qu'une sonde sourde ne fasse pas s'accumuler les goroutines), `/metrics` depuis le registre gardé ; aucun corps ne porte une
  erreur de sonde ou de collecteur, qui peut porter un hôte : `/metrics` sert ce qu'il a pu collecter, et
  compte les collectes en erreur dans `ops_metrics_gather_errors_total`, pour qu'une panne ne se réduise
  pas à des séries disparues.
  `ReadHeaderTimeout` et `IdleTimeout` à 2 s ; pas de `WriteTimeout`, le seul lecteur lent possible est
  Prometheus, derrière la NetworkPolicy. Une sonde : `ops.Check{Name, Probe}`, nom unique et non vide. L'arrêt laisse finir les requêtes en
  vol, en `SHUTDOWN_TIMEOUT` au plus. Pas de `pprof`.

## Design arrêté

- `internal/platform/config` : `Infra`, `Load() (Infra, error)`.
- `internal/platform/supervisor` : `Ordered{DrainDelay, DrainBudget time.Duration}`, `Add(name string,
  fn func(context.Context) error)`, `OnDrain(func())`, `Run(context.Context, *slog.Logger) error`,
  `ErrDrainBudgetExceeded`.
- `internal/platform/ops` : `Check{Name string; Probe func(context.Context) error}` ; `New(addr string,
  shutdown time.Duration, prometheus.Gatherer, ...Check) (*Server, error)`, qui écoute dès la
  construction ; `(*Server) Drain()` ; `(*Server) Run(context.Context) error`, un composant du
  superviseur qui s'arrête en `SHUTDOWN_TIMEOUT` au plus ; `(*Server) Addr() string`.
- `.golangci.yml` : forbidigo sur la lecture d'environnement, exclusion du chargeur, de `tools/catalog` et
  de `tools/debtguard`.
- Vérification du critère : chaque variable lue, défaut appliqué, valeur illisible (une erreur) et règle
  violée, ratio 0 et 1 admis ;
  une lecture d'environnement hors du paquet rend `make lint` rouge ; les rappels avant tout arrêt, le
  délai respecté, l'ordre inverse, le budget global, hors délai, qui nomme le composant bloqué et annule les
  suivants, la première erreur avant le dépassement, le drain sur erreur et sur `nil` prématuré ; `/healthz`, `/readyz` prêt, en échec de sonde, sous délai et en
  drain, une sonde sourde lancée une seule fois et rétablie à son retour, `/metrics` servi sans erreur de
  collecteur et qui la compte, l'arrêt borné et la connexion inactive fermée, noms de sonde vides ou en double refusés.
