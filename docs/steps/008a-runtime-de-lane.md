# Step 008a — Runtime de lane : lane `group`, dead-letter, gardes

Plan : `docs/plan/composants.md` §008, première des deux PR (008a, 008b). Spec : `docs/spec/09-contrats.md` §9.1
(groupes, en-têtes communs, gardes 1 à 3 et 6), `docs/spec/02-architecture.md` §2.3 et §2.4, `docs/spec/10-exploitation.md`
§10.1 (alertes). Décision : `docs/design/04-contrats-de-topics-et-lanes.md` §3.4, §4 à §8. Précédent :
`docs/steps/004a-catalogue-des-topics.md` (le catalogue, les reports des gardes qui lisent le code). Règles :
`.claude/rules/topics.md`, `.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- Le catalogue (`internal/catalog`) déclare 20 topics et 38 groupes : modes de dépendance, tenant, concurrence,
  `max_age`, dead-letter, `start`, `assignment`. Aucun code ne le consomme encore, hors `catalog-apply` (topics, ACL).
- Aucun service n'existe : le runtime naît sans consommateur réel. Ses consommateurs arrivent avec 021 (production
  seule), 022 (routeur : `blocking`, `park_global`, `broadcast`), 023a (pool : `manual`, `fail_open`), 024
  (`message-state-svc` : `blocking`), 029b et 030 (`park_by_tenant`, `per_tenant`).
- Source du portage : `go-gateway/internal/storage/kafka/consumer.go` — lot et barrière, commit du préfixe traité,
  rejeu en place avec backoff borné et compteur d'essais sans progrès (step-285), commit détaché au drain,
  enregistrements retenus entre deux appels, lag par partition qui refuse un total mensonger.
- La spec 09 §9.1 ne connaît que quatre en-têtes (`origin_ts`, `trace_id`, `replay_count`, `replayed_at`) : « toute
  autre donnée est un champ du payload ». La décision 4 §4.6 fait porter à un record de dead-letter sa raison, son
  instant et sa position source. `DeadLetters()` du catalogue laisse aujourd'hui le payload vide.
- La règle `topics` désigne `internal/lane/**` ; le plan écrivait « sous `internal/platform/` ».
- Dettes du territoire : #69 (rejeu en place qui continue sur une partition réattribuée) est le comportement porté ;
  #64 et #47 touchent le pool (023a) ; #77 un test du routeur (022).

## Arbitrages

Fable, 2026-10-19 ; aucun désaccord laissé à l'humain.

1. **Deux PR.** 008a : la lane `group`, les issues, `blocking` et `fail_open`, l'âge, la dead-letter, le producteur,
   les métriques, les gardes 1 à 3 et 6. 008b : `park_by_tenant`, `park_global`, `per_tenant`, le test de
   non-régression de la décision 4 §8. 008a **refuse à la construction** un groupe qui déclare un mode ou une
   concurrence de 008b, jamais ne le dégrade en `blocking`. Le « Terminé quand » du plan (I11) se ferme en 008b.
2. **Emplacement** : `internal/lane` (cœur sans I/O : issues, âge, en-têtes, politique), `internal/lane/kafka`
   (franz-go). Le plan est corrigé.
3. **Dead-letter enveloppée** : `sms.v1.DeadLetter` (payload et en-têtes d'origine, raison, instant, topic,
   partition, offset), gardé par `buf breaking`. Le record de dead-letter recopie aussi les quatre en-têtes
   communs. La raison est un code court, jamais le texte d'une erreur (I1). `DeadLetters()` nomme le payload,
   `MissingPayloads` couvre les dead-letters, §9.1 le dit. `replay-dead-letter` (027) désenveloppe.
4. **Âge maximal** : plafond du catalogue (D3 par catégorie, ou durée fixe) depuis `origin_ts` ; la catégorie se lit
   au suffixe du topic, sinon la lane la fournit (`mt.reroute-park`) et la construction échoue sans elle ; la lane
   peut abaisser la limite d'un record (`validity_period`, surcharge client), jamais la relever.
5. **`assignment`** : 008a ne sert que `group` ; `broadcast` et `manual` sont un ordonnancement du plan (022, 023a,
   amendés dans cette PR), pas un report : le refus à la construction cite la step.
6. **Gardes.**
   - 1 et 2, par service : pour chaque `cmd/<service>`, tout groupe et tout topic produit du service a son appel, et
     tout appel a son entrée ; un service du catalogue sans `cmd/` est listé dans la sortie du test. Le topic d'un
     appel est un littéral ou une famille par catégorie, que la garde résout en ses trois topics.
   - 2 vaut aussi à l'exécution : le producteur refuse un topic où son service ne figure pas.
   - 3 : une `Transient` ou une `Degraded` sur une dépendance non déclarée, ou d'un autre mode, est une rupture de
     contrat : la lane s'arrête en erreur, sans commit (comme `committablePrefix`). Kafka est le runtime lui-même.
   - 6 : `deploy/guard_test.go`, prouvée rouge sur une fixture (HPA au-delà des partitions).
7. **Production vers la dead-letter** : synchrone et acquittée avant le commit ; en échec, elle est `blocking`
   (décision 4 §7).
8. **Métriques** : un seul compteur, `lane_records_total{group,outcome}` (`ok`, `refused`, `degraded`, `poison`,
   `expired`) : il porte l'écart `fail_open` (`degraded`) et la règle « dead-letter qui reçoit » (`poison`), qui
   entre ici. Le lag par lane, sa règle (seuil propre à la lane, OTP) et son `absent()` entrent avec le premier
   service qui consomme, 022 : une jauge sans consommateur déployé ni règle ne se vérifie pas.
9. **Dettes** : #69 reportée, sa marque suit le rejeu porté dans `internal/lane/kafka` ; #64 et #47 en 023a, #77 en
   022 : hors du territoire touché ici.

## Design arrêté

- **`contracts/proto/sms/v1/deadletter.proto`** : `DeadLetter { bytes payload; repeated Header headers; string
  reason; google.protobuf.Timestamp at; string topic; int32 partition; int64 offset }`, `Header { string key; bytes
  value }`. `internal/catalog` : `DeadLetters()` pose `Payload: "sms.v1.DeadLetter"` ; `MissingPayloads` les couvre ;
  `docs/topics.md` régénérée ; spec 09 §9.1 nomme le payload.
- **`internal/lane`** (cœur, sans I/O) :
  - `Outcome` et ses constructeurs `Ok()`, `Refuse(code errors.Code)`, `Transient(dep string, err error)`,
    `Poison(reason string)`, `Degraded(dep string)` (`fail_open` : fait, sans la dépendance, compté).
  - `Headers` : `OriginTS`, `TraceID`, `ReplayCount`, `ReplayedAt` ; `origin_ts` en RFC 3339 nanosecondes UTC ;
    `ParseHeaders` refuse un `origin_ts` absent ou illisible (le record est un poison).
  - `Record` : topic, partition, offset, clé, valeur, `Headers`, en-têtes bruts.
  - `Policy` : bâtie depuis un `catalog.Group` et son topic ; refuse un `assignment` autre que `group`, un mode
    `park_*`, `per_tenant` (en citant 022, 023a, 008b) ; `MaxAge(rec, lower)` ; `Mode(dep)`.
- **`internal/lane/kafka`** :
  - `NewLane(cat, group, Handlers, log, opts...)` : groupe du catalogue, son topic, `DisableAutoCommit`, `start` →
    `ConsumeResetOffset`, prouvé par comportement. `Handlers` : `Handle`, `Expired`, et facultatifs `Category`,
    `Limit`.
  - `Run(ctx)` : boucle portée, un lot et un traitement par record. Âge dépassé → `Expired` ; `Ok`, `Refuse`,
    `Degraded` → suivant, compté ; `Poison` → dead-letter synchrone puis suivant ; `Transient` d'une dépendance
    `blocking` → commit du préfixe, rejeu en place. Commit détaché borné au drain. Pas de records retenus entre deux
    `Run` : un service lance un `Run` par lane, puis la ferme (le pool qui rappelait la boucle de `go-gateway` n'a
    plus ce besoin). Une attente
    `blocking` retient tout le lot du pod : la dépendance est globale par définition.
  - Aucun journal ni span ne porte le texte d'une erreur du traitement ni la valeur d'un record : le code (`errors.Code`)
    ou `internal_error`.
  - `NewProducer(cfg, cat, service, topics...)` : refuse un topic où `service` ne figure pas ; `Produce` refuse un
    topic hors de sa liste, et un record sans `origin_ts`.
  - `Register(prometheus.Registerer)` : `lane_records_total{group,outcome}`, compté quand un record est fait ; le
    service l'enregistre sous `metrics.Guard`.
- **Gardes** : `internal/lane/kafka/guard_test.go` (1 et 2, AST sur `cmd/` et `internal/`, appels `NewLane` et
  `NewProducer` à arguments littéraux ou familles `catalog.Category(…)`) ; garde 6 dans `deploy/guard_test.go`.
- **Alerte** : `deploy/alerts/lanes.yaml`, `LaneDeadLetterReceiving` sur
  `sum by (group) (increase(lane_records_total{outcome="poison"}[5m])) > 0`, et son test promtool.
- **Plan** : §008 coupée en 008a et 008b, `internal/lane` ; §022 nomme `broadcast`, §023a `manual` et `fail_open`.
- **Tests bloquants** (décision 4 §8, part de 008a), sur Kafka testcontainers : un poison part en dead-letter sans
  bloquer sa partition ; le record de dead-letter garde `origin_ts` et ses quatre en-têtes ; un record expiré n'est
  jamais traité ; une dépendance `blocking` en échec rejoue en place sans commit ; un drain commite ce qui est fait ;
  aucun corps dans le journal, la raison ni l'erreur (I1) ; chaque refus de construction et chaque rupture de
  contrat.
