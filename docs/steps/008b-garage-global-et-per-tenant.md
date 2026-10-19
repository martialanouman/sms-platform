# Step 008b — Runtime de lane : garage global, `per_tenant`

Plan : `docs/plan/composants.md` §008, seconde PR du runtime de lane. Spec : `docs/spec/09-contrats.md` §9.1,
`docs/spec/04-voie-retour.md` §4.4, `docs/spec/03-pipeline-mt.md` (étape 8, crédit). Décision :
`docs/design/04-contrats-de-topics-et-lanes.md` §3.2, §4.3, §4.5, §5.2, §5.3, §7, §8 ;
`docs/design/01-modele-credit-enveloppes.md` §7 (garage). Précédent : `docs/steps/008a-runtime-de-lane.md`.
Règles : `.claude/rules/topics.md`, `.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 008a sert la lane `group`, `blocking`, `fail_open`, l'âge, la dead-letter ; elle refuse à la construction
  `park_by_tenant`, `park_global` et `per_tenant` en citant 008b.
- Deux familles de groupes les déclarent : `router-svc` sur `mt.inbound.<cat>` (`billing-svc: park_global`,
  consommateur en 022) ; `delivery-svc` sur `dlr.routed` et `mo.routed` (`endpoint: park_by_tenant`,
  `per_tenant`, `max_in_flight: 8`, consommateur en 030).
- Le catalogue ne dit pas vers quel topic gare un `park_global` ; `mt.billing-wait.<cat>` a `producers:
  [router-svc]`, et la garde 1 exigera du routeur un `NewProducer` qu'il n'écrira jamais : c'est le runtime qui gare.
- La clé de `mt.billing-wait.<cat>` : `message_id` en spec 09 §9.1, « par propriétaire » en décision 1 §7.
- Dettes du territoire : #69 (rejeu en place sur une partition réattribuée), #33 (produces des tests Kafka).

## Arbitrages

Fable, 2026-10-19, sauf les deux points tranchés par l'humain.

1. **Coupe.** 008b : `park_global`, `per_tenant`, le volet p99 du test de non-régression (décision 4 §8) ;
   rien n'y exige le garage par tenant. **`park_by_tenant` (`lane_parking`, disjoncteur, sonde, vidange) passe
   en 029c, juste avant 030, son consommateur** (humain, 2026-10-19) : concevoir le tenant et la sonde contre
   le vrai traitement de `delivery-svc`. I11 se ferme en 029c ; le plan est amendé. `NewPolicy` refuse
   `park_by_tenant` en citant 029c.
2. **Topic de garage déclaré au catalogue** : `billing-svc: {mode: park_global, park: mt.billing-wait.otp}`.
   Le catalogue exige `park` pour `park_global` et seulement pour lui, un topic déclaré, le service du groupe
   parmi ses `producers`, et le même payload que le topic source (la lane d'attente rejoue avec le même
   traitement). La garde 1 compte le `park` d'un groupe consommé comme produit par son service. Spec 09 §9.1 et
   décision 4 §3.2 nomment le champ.
3. **Clé du garage : celle du record source** (`message_id`), spec 09 fait foi (humain, 2026-10-19) ;
   `order: none`, la clé n'ordonne rien. Décision 1 §7 amendée.
4. **Garer** : un `Transient` sur une dépendance `park_global` produit le record (clé, valeur, en-têtes tels
   quels, `origin_ts` compris, décision 4 §4.5) sur le topic de garage, synchrone et acquitté avant le commit ;
   en échec, `blocking` sur Kafka (décision 4 §7), comme la dead-letter. Compté `parked`.
5. **Tenant** : `Handlers.Tenant func(Record) (string, error)`, exigé à la construction d'un groupe
   `per_tenant` ; le runtime ne lit pas le payload. En erreur, le record est un poison `unreadable_tenant`.
6. **`per_tenant`** (décision 4 §5.3, spec 04 §4.4) :
   - une file FIFO par tenant, au plus `max_in_flight` traitements en vol par tenant ; l'ordre n'est pas garanti
     dans cette fenêtre ;
   - commit du préfixe terminé de chaque partition, jamais au-delà du plus ancien record non terminé ;
   - pas de barrière de lot : la lecture continue tant que la fenêtre non commitée du pod tient sous 1 024
     records. Un client lent qui la sature retient tout le pod : plafond assumé (« le garage ne le couvre pas »),
     marqué `ponytail:` ;
   - pas de plafond global de tenants : la fenêtre borne déjà la mémoire.
7. **`blocking` sous `per_tenant`** : le record rejoue en place dans son worker, avec le backoff de 008a ; le
   commit reste au plus ancien non terminé. Une dépendance globale en panne fait échouer tous les tenants, tout
   s'arrête de soi-même ; une panne partielle laisse passer ce qui réussit. Choix de mise en œuvre (Fable
   proposait d'arrêter tout envoi) : il ne contredit pas la spec et évite une seconde machine d'état.
8. **Dettes** : #69 reportée, sa marque suit aussi le rejeu du worker ; #33 reportée, les tests ajoutés suivent
   le motif existant.

## Design arrêté

- **Catalogue** : `Dependency.Park` (`yaml:"park"`) et ses validations (arbitrage 2) ; `contracts/topics.yaml`,
  les trois `router-svc` `mt.inbound.<cat>` déclarent leur `park` ; page générée si elle montre les dépendances.
- **`internal/lane`** : `Policy` accepte `park_global` et `per_tenant`, refuse `park_by_tenant` (029c) ;
  `Park(dep)`, `MaxInFlight()` (positif pour un groupe `per_tenant`, que le catalogue l'impose).
- **`internal/lane/kafka`** :
  - `Handlers.Tenant`, exigé pour `per_tenant`.
  - Un seul règlement d'issue par record, partagé par les deux modes : `Ok`, `Refuse`, `Degraded` comptés ;
    `Poison` → dead-letter ; `Transient` `park_global` → garage ; `Transient` `blocking` → rejeu en place ;
    rupture de contrat → arrêt après commit du fait.
  - `per_partition` : la boucle de 008a, inchangée hors règlement.
  - `per_tenant` : une goroutine de lecture et un coordinateur ; fenêtre de 1 024 ; commit du préfixe contigu
    terminé après chaque tour ; au drain, attente des workers puis commit détaché. Une rupture de contrat, une
    faute de lecture ou de commit annule les workers en vol, rendus non terminés : la lane s'arrête même quand un
    autre tenant rejoue en place.
  - `lane_records_total{outcome="parked"}`.
- **Garde 1** : le `park` d'un groupe consommé compte comme produit par son service.
- **Spec et décisions** : spec 09 §9.1 (`park`), décision 4 §3.2, décision 1 §7 (clé) ; plan §008 et §029c.
- **Tests bloquants**, Kafka testcontainers :
  - un `Transient` `park_global` arrive sur le topic de garage, même clé, mêmes en-têtes, et la lane continue ;
  - garage en échec : rien n'est commité ;
  - non-régression (décision 4 §8, volet p99) : 24 comptes en `Refuse(no_receiver)` et un compte à endpoint lent
    à côté d'un compte normal, sur une partition : **p99 de traitement du compte normal < 1 s** depuis
    `origin_ts` ;
  - au plus `max_in_flight` en vol par tenant ; commit jamais au-delà du plus ancien non terminé ;
  - drain sous `per_tenant` : ce qui est fait est commité, le reste relu ;
  - `blocking` sous `per_tenant` : rejeu en place sans commit au-delà ;
  - refus de construction : `park_by_tenant` (029c), `per_tenant` sans `Tenant` ; catalogue : `park` absent,
    superflu, topic inconnu, service non producteur, payload différent.

## Revue

Trois axes en lecture seule : correction et concurrence, conformité à la spec, code en trop. Un tour.

**Corrigé** (rouge lu, mutation tuée) :

- une rupture de contrat, une faute de lecture ou de commit n'arrêtait pas une lane `per_tenant` tant qu'un autre
  tenant rejouait en place : les workers sont annulés à la rupture (relevé par deux axes, prouvé par exécution) ;
- trouvé au TDD : la fenêtre ne bornait que des lots entiers, un seul fetch pouvait en dépasser la taille ; un lot
  est admis record par record ;
- le test de drain prouve aussi que ce qui est fait avant le record non terminé est commité ;
- supprimés : le compteur `running` (redondant avec `inFlight`), deux garages de la fixture de la garde, une
  assertion contre la valeur zéro, un test fondu dans celui des traitements requis, des copies de traitement ;
- la marque `debt: #33` posée sur `send`, où vit le report ; le commentaire `ponytail:` cite l'arbitrage 6, pas
  la décision 4 §5.3 ; le renvoi de 008a vers 029c.

**Laissé, avec sa raison** :

- *La décision 4 §5.3 promet qu'« un client lent ne ralentit que sa file »* ; la fenêtre de 1 024 records non
  commités la borne : au-delà, le pod entier attend le client lent. Une fenêtre sans borne garderait en mémoire
  tout l'arriéré d'un client lent. Point remonté à l'humain dans la PR.
- *La garde de `pctx.Err()` du lecteur doublonne le `select`* : elle évite un tour à vide, gain nul à la couper.
