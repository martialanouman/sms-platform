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
   par `jsonb_object_agg` comme le trigger. Ligne absente : suppression. Table non chargée : ignorée.
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
  - `Open(ctx, log, dsn string, tables ...Table) (*Snapshots, error)` ; `(*Snapshots).Run(ctx)`
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

## Correctifs de revue, tour 1

Trois revues (correction, tests creux, code en trop) ; Fable, 2026-10-19.

1. **Clé primaire modifiée** : `audit_capture` ne notifiait que la nouvelle clé, l'ancienne ligne restait
   jusqu'à la resync. Migration `00002` (`CREATE OR REPLACE`, propriétaire et ACL gardés) : un `UPDATE` qui
   change la clé notifie aussi l'ancienne. Deux notifications, une clé chacune (D1). `audit_changes` inchangé.
2. **Clé introuvable** : `Open` refuse une table dont une colonne de clé est retirée par le trigger
   (`*_sealed`, `*_hash`, `*_kms_key_ref`, `wrapped_key` : il notifierait une clé nulle) ou de type
   `timestamptz`/`timetz` (`to_jsonb` les rend dans le fuseau de la session : la clé de l'écrivain ne serait
   pas celle du lecteur).
3. **Borne** : 10 s par chargement (resync d'une table, relecture d'une ligne) ; l'échec fait reconnecter.
   Une coupure silencieuse se voit au plus à la resync suivante (5 min 10 s).
4. **Fraîcheur** : une ligne est observée si son commit est postérieur au plus récent commit vu dans la
   table avant ce chargement ; le maximum est remplacé après. Un `ADD COLUMN` ou `DROP COLUMN` ne touche pas
   `xmin` et ne fait plus partir l'alerte ; une réécriture complète (`ALTER TYPE`) la fait partir une fois,
   accepté. Plus de comparaison de JSON ni de copie de la table.
5. **Simplifications** : « observer » et « garder la vue en cas d'échec » valent « la table est déjà
   chargée » ; la connexion se ferme quand `Open` échoue ; `!ok` du découpage de la charge utile retiré
   (`json.Valid("")` la refuse déjà).
6. **Tests** : valeur observée (≥ durée d'une coupure, notification perdue) ; lecture de la vue pendant la
   coupure ; resync périodique sous un flot de notifications ; `LISTEN` avant le chargement ; ni `Build` en
   échec ni `*` ne resynchronisent ; journal d'une charge utile illisible sans elle ; témoin de colonne non
   scellée ; clé composite de la sonde ; `Open` derrière `pgtest.Late` ; motifs d'échec d'`Open` ; alerte
   resserrée autour de 5 s ; aucune connexion laissée par un `Open` en échec.
7. Écarté : un horodatage NULL n'arrive qu'au-delà de la rétention des commit timestamps, intestable ; la
   condition reste, sans test ni dette.
