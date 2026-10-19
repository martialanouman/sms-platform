# Step 009 — Instantané de configuration

Plan : `docs/plan/composants.md` §009. Spec : `docs/spec/07-plan-de-controle.md` §7.1, §7.2 ;
`docs/spec/10-exploitation.md` §10.1 (alertes). Décision : `docs/design/07-distribution-de-config-et-redis.md`
§2, §3, §5. Précédents : `docs/steps/005e-tests.md` (`tcpproxy`), `docs/steps/006a-audit.md` (trigger).
Règles : `.claude/rules/go-code.md`, `.claude/rules/tests.md`, `.claude/rules/ci-deploy.md`. Mode : PR.

## Contexte

- Le trigger `control_plane.audit_capture()` émet déjà `pg_notify('config', '<table>:<pk>')`, où `<pk>`
  est l'objet jsonb des colonnes de la clé primaire, délivré au commit.
- Aucune table de configuration n'existe avant 014a, ni aucun service avant 011a : la bibliothèque se
  prouve sur une table sonde créée par le test.
- Le trigger par instruction `<table>:*` naît avec le premier import de masse ; cette step le consomme.
- Dettes du territoire : aucune issue `debt` ouverte ne touche la distribution de configuration.

## Arbitrages

Fable, 2026-10-19, sauf les deux points tranchés par l'humain.

1. **Emplacement** : `internal/platform/snapshot` (`platform/config` lit l'environnement).
2. **Forme** : la bibliothèque garde les lignes brutes (`to_jsonb`) par table et par clé ; le
   consommateur déclare `Table{Name, Build func(rows []json.RawMessage) (any, error)}`. `Build` est
   rejoué sur la table entière à chaque changement ; son résultat est ce que rend l'instantané. Aucun
   décodage sur le chemin chaud. `Build` en échec : `Open` échoue ; ensuite, la vue précédente reste.
3. **Liste des tables** : passée par le consommateur. Le GRANT n'est pas la liste de l'instantané
   (`audit_log`, `exact_routes`) ; une table non déclarée fait échouer `Open` faute de GRANT (garde 1).
4. **Boucle** : une connexion directe, une goroutine. `LISTEN config` avant le chargement ; colonnes de
   la clé lues dans `pg_index` à l'ouverture ; une seule requête pour charger et relire, la clé rendue
   par `jsonb_build_object` comme le trigger. Ligne absente : suppression. Table non chargée : ignorée.
   Charge utile illisible : journalisée sans la charge utile et ignorée (`NOTIFY` n'exige aucun
   privilège ; la resync de 5 min rattrape).
5. **Colonnes scellées** : `*_sealed`, `*_kms_key_ref` et `wrapped_key` retirées à la lecture (spec
   §7.1) ; `*_hash` gardé, vérifié par l'ingress (humain, 2026-10-19).
6. **Démarrage et pannes** : `Open` bloque jusqu'au premier instantané (composition par `pgdial.Retry`) ;
   `Run` sert la boucle. Reconnexion toutes les secondes, resync complète à chaque reconnexion et
   toutes les 5 min. Postgres injoignable : l'instantané courant sert.
7. **Fraîcheur** : `track_commit_timestamp = on` sur le cluster (humain, 2026-10-19 ; redémarrage) ;
   `pg_xact_commit_timestamp(xmin)` lu avec la ligne. Histogramme `config_propagation_seconds`, sans
   label, observé pour chaque ligne dont le JSON change, sur notification comme en resync : une
   notification perdue se voit en minutes. Suppression ou horodatage NULL : pas d'observation.
   Paramètre absent : Postgres ne lève d'erreur que sur une table non vide ; la garde de la brique
   tient le paramètre (amendé à l'implémentation). Pas de jauge d'âge de resync.
8. **Cuttable** : `pgtest.DB.Cuttable(t) (DB, *tcpproxy.Proxy)`, miroir de `Late`.
9. **Dettes** : aucune ouverte, aucune reportée.

## Design arrêté

- **`internal/platform/snapshot`** :
  - `Table{Name string; Build func([]json.RawMessage) (any, error)}` ;
  - `Open(ctx, log, dsn string, tables ...Table) (*Snapshots, error)` ; `(*Snapshots).Run(ctx) error`
    jusqu'à l'annulation ; `(*Snapshots).Get(table string) any`, lu par `atomic.Pointer` ;
  - `Register(prometheus.Registerer) error` pour `config_propagation_seconds` (bornes 0.1, 0.5, 1, 2,
    5, 10, 30, 60, 300) ;
  - période de resync en variable de paquet, abaissée par un test interne.
- **`pgtest`** : `Cuttable` ; le conteneur démarre avec `-c track_commit_timestamp=on`.
- **Déploiement** : `postgresql.parameters.track_commit_timestamp: "on"` dans
  `deploy/bricks/postgres.yaml`, gardé par `deploy/guard_test.go`.
- **Alerte** : `deploy/alerts/config.yaml`, `ConfigPropagationSlow` =
  `histogram_quantile(0.99, sum by (le) (rate(config_propagation_seconds_bucket[5m]))) > 5`, et son
  `config_test.yaml` (part, reste muette).
- **Tests bloquants**, Postgres testcontainers, table sonde avec `audit_capture` et une colonne
  `*_sealed`, lue par un rôle qui n'a que `SELECT` :
  - une écriture sous `app.actor` arrive dans l'instantané, sans sa colonne scellée ; une suppression
    en sort ;
  - écoute coupée par le proxy, écriture par un client non coupé, reprise : la ligne arrive (resync) ;
  - écriture trigger désactivé, puis `pg_notify('config', 'probe:*')` : la table est rechargée ;
  - écriture trigger désactivé, sans notification : la resync périodique l'amène ;
  - `config_propagation_seconds` observé pour l'écriture et la mise à jour, ni au chargement, ni à la
    suppression, ni pour une ligne inchangée en resync ;
  - `Open` échoue sur une table sans GRANT, sans clé primaire, ou dont `Build` échoue ; un `Build` en
    échec ensuite garde la vue précédente sans bloquer les autres tables ;
  - une charge utile illisible ou d'une table non tenue est ignorée, sans resync.
