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
  fermée des variables. Écrit à la main (`os.LookupEnv`, `time.ParseDuration`, `strconv`), sans
  dépendance. Seuls les champs qui ont un consommateur : `LOG_LEVEL` (`info`), `OPS_ADDR` (`:9090`),
  `DRAIN_DELAY` (5 s), `DRAIN_BUDGET` (90 s), `SHUTDOWN_TIMEOUT` (30 s), `TLS_CERT_FILE`, `TLS_KEY_FILE`,
  `TLS_CA_FILE`, `TLS_ALLOWED_CLIENTS` (liste séparée par des virgules), `TRACE_SAMPLE_RATIO` (1, la queue
  est au collecteur). Ni `Environment` (aucun usage depuis la 005c), ni endpoint OTLP (dette #93, au premier
  `cmd/`), ni nom de service (une constante du binaire).
- **Garde** : forbidigo refuse `os.Getenv`, `os.LookupEnv`, `os.Environ`, `os.ExpandEnv` et
  `syscall.Getenv` hors de `internal/platform/config`. Distinguer `DRAIN_DELAY` de `MAX_BINDS` est un
  jugement de revue, pas d'analyseur : un champ ajouté à la structure passe en revue.
- **Validation** : `DRAIN_DELAY` ≥ 0, `SHUTDOWN_TIMEOUT` > 0, `DRAIN_BUDGET` > `SHUTDOWN_TIMEOUT` (un
  composant a droit à tout son délai, et le drain est séquentiel), ratio dans [0, 1], toute valeur
  illisible refusée ; les erreurs sont jointes pour qu'un démarrage les montre toutes. Rien sur les
  fichiers TLS ni l'allowlist, que `mtls` refuse déjà à la construction ; rien sur `OPS_ADDR`, que
  `net.Listen` refuse mieux.
- **Superviseur** : `supervisor.Ordered{DrainDelay, DrainBudget}` seul. `Run` : à l'arrêt demandé ou au
  premier retour d'un composant (une erreur, ou un `nil` prématuré, rendu `"<nom>: stopped before
  drain"`), les rappels `OnDrain`, puis `DRAIN_DELAY`, puis l'arrêt des composants **dans l'ordre inverse
  de leur ajout**, un à la fois, sous `DRAIN_BUDGET` ; au-delà, le composant attendu est nommé et les
  suivants sont annulés sans attente. `Run` rend la première erreur, sinon `ErrDrainBudgetExceeded`,
  sinon nil. Amendé : spec 10 §10.2 (« dans l'ordre inverse de leur enregistrement »). Les gardes de
  go-gateway sur les `cmd/` viennent avec le premier `cmd/` : elles ne garderaient rien ici.
- **Serveur ops** (`internal/platform/ops`) : HTTP en clair sur `OPS_ADDR`, hors mTLS, borné par la
  NetworkPolicy au kubelet et à Prometheus (décision humaine ; amendé : décision 6 §4, spec 02 §2.5).
  `/healthz` (200 tant que le processus vit), `/readyz` (503 `draining` dès le drain, sans sonder ; sinon
  les sondes vitales en parallèle sous un délai de 2 s, 503 et le nom de la sonde en échec), `/metrics`
  depuis le registre gardé. Une sonde : `ops.Check{Name, Probe}`, nom unique et non vide. Pas de `pprof`.

## Design arrêté

- `internal/platform/config` : `Infra`, `Load() (Infra, error)`.
- `internal/platform/supervisor` : `Ordered{DrainDelay, DrainBudget time.Duration}`, `Add(name string,
  fn func(context.Context) error)`, `OnDrain(func())`, `Run(context.Context, *slog.Logger) error`,
  `ErrDrainBudgetExceeded`.
- `internal/platform/ops` : `Check{Name string; Probe func(context.Context) error}` ; `New(addr string,
  prometheus.Gatherer, ...Check) (*Server, error)`, `(*Server) Drain()`, `(*Server) Run(context.Context)
  error` (un composant du superviseur), `(*Server) Addr() string`.
- `.golangci.yml` : forbidigo sur la lecture d'environnement, exclusion du paquet `config`.
- Vérification du critère : chaque variable lue, défaut appliqué, valeur illisible et règle violée ;
  une lecture d'environnement hors du paquet rend `make lint` rouge ; les rappels avant tout arrêt, le
  délai respecté, l'ordre inverse, le budget qui nomme le composant bloqué et annule les suivants, le drain
  sur erreur et sur `nil` prématuré ; `/healthz`, `/readyz` prêt, en échec de sonde, sous délai et en
  drain, `/metrics` servi, noms de sonde vides ou en double refusés.
