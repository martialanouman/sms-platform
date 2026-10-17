# Step 004a — Catalogue des topics

Plan : `docs/plan/socle.md` §004, premier tiers (découpage en 004a, 004b, 004c décidé par l'humain le
2026-10-17 : la step entière dépassait une PR relisible). Spec : `docs/spec/09-contrats.md` §9.1.
Décision : `docs/design/04-contrats-de-topics-et-lanes.md`. Mode : PR.

## Contexte

`contracts/topics.yaml` déclare chaque topic et chaque groupe ; le runtime de lane (plus tard) et le Job
d'application (step 007) le liront. Cette step pose le fichier, son chargeur typé, les gardes statiques
et la page de documentation générée. Les gardes qui lisent le code (1 à 3, 6) viennent avec le runtime,
l'ACL (7) avec le Job.

**Terminé quand** (part de 004a) : le catalogue reproduit le chapitre 9 §9.1 ; chaque garde statique est
rouge sur une entrée fautive.

## Arbitrages

1. **Partitions** (Fable, la spec ne donnant que des contraintes) : 64 au moins pour tout topic consommé
   par groupe sous HPA, la garde 6 bornant `maxReplicas` par le plus petit topic consommé ; 128 pour
   les gros `per_key`, figés à vie (`mt.outcome`, `message.state`) ; 384 par `mt.routed.<cat>` (~320
   comptes, partition manuelle, extensible par blocs) ; 8 pour `connector.account-status` (lu en entier
   par chaque pod) et pour chaque dead-letter (rejeu manuel, pas d'HPA).
2. **Rétention 96 h partout**, dead-letters comprises. Fable proposait 30 jours pour les dead-letters
   sans corps ; la spec 05 §5.4 borne aussi le **MSISDN** en transit à 96 h, et `message.state` comme
   les DLR portent des adresses. Le topic compacté n'a pas de rétention.
3. **Champ de groupe `assignment`** (choix de l'humain, recommandé par Fable) : `group` (défaut),
   `broadcast` (chaque pod lit tout : routeur sur `connector.account-status`), `manual` (pool sous bail
   sur `mt.routed.<cat>`). Les deux derniers sortent de la garde 6. La spec 09 §9.1 est amendée.
4. **Un groupe par (service, topic)**, nommé `<service>.<topic>`, dead-letter `<groupe>.dead-letter`
   (choix de l'humain) : le pool a trois groupes, un par catégorie, et le design 3 §4.3 (« un groupe
   unique `connector-pool` ») est amendé. Le runtime du pool ordonnance pourtant ses trois topics
   ensemble (ADR-0021) : trois déclarations, un ordonnanceur.
5. **Les dead-letters ne sont pas déclarées comme topics** : le chargeur les dérive de chaque groupe
   (clé et corps de la source, ordre `none`, 8 partitions, 96 h). Le champ `dead_letter` reste
   obligatoire et doit valoir `<groupe>.dead-letter` : la garde voit un groupe qui l'oublie ou le
   détourne.
6. **Justifications** : `start: latest` exige `start_justification` ; une dépendance `fail_open` exige
   sa `justification`. Seul le jeton d'annulation du routeur (ADR-0013) en porte une.
7. **Partitions figées** : la garde compare chaque topic `per_key` à sa version sur la branche de base
   (`git show origin/main:contracts/topics.yaml`). Une base sans catalogue n'impose rien ; une base
   introuvable est une erreur. La CI récupère l'historique (`fetch-depth: 0`).
8. **Clean architecture** : le cœur `internal/catalog` (types, `Parse`, `Validate`, `Frozen`,
   `DeadLetters`, `Markdown`) importe `go.yaml.in/yaml/v3`, bibliothèque pure ajoutée à la liste
   blanche depguard ; `tools/catalog` compose (fichier, git, écriture de la page).
9. **Payloads** : le champ `payload` nomme le message protobuf que la step 004b créera
   (`sms.v1.<Nom>`) ; 004b ajoutera la garde « chaque payload existe ».
10. **Inventaire RGPD** : la garde 4 de la spec exige aussi l'inscription du topic à l'inventaire RGPD,
    qui n'existe que comme tableau de la spec 05, illisible par une machine. Report : une issue `debt`
    dans cette PR, marquée dans `Validate`.

11. **Revue, premier tour** : trois défauts de contenu.
    - La lane MO du routeur dépend aussi de `redis-state` (réassemblage) et de `control-plane` (STOP), en
      `blocking` (spec 04 §4.6).
    - La projection CDR lit le corps sur `mt.inbound.*` et `mo.inbound` (spec 05 §5.2) : `message-state-svc`
      a deux lanes sur ces topics. Champ facultatif `lane`, groupe `<service>.<lane>.<topic>` ; la règle
      choisie par l'humain reste le défaut, elle s'étend. §9.1 le nomme.
    - L'anti-spam du routeur appelle `redis-state` sans mode de panne écrit dans la spec : dette ouverte
      #29, marquée sur les lanes du routeur, au lieu d'un mode inventé. `redis-cache` n'est pas une
      dépendance de lane : accélérateur des lectures `control-plane` (décision 7), repli direct sur
      Postgres.
    - La remise appelle `session-manager-svc` et `smpp-server-svc` sous la dépendance `endpoint` (un bind
      introuvable est un endpoint en panne, spec 04 §4.6).
12. **Revue, second tour** : `content-key-svc` ne concerne que le scellement du corps (groupes de
    projection sur `mt.inbound.*` et `mo.inbound`), pas la projection de `message.state` : sa panne
    retarde le contenu, pas le CDR (spec 05 §5.5). Le scellement lit aussi `customers.content_storage` et
    le `customer_id` dans `control-plane`, `blocking`. Une clé changée sur un topic `per_key` casse
    l'ordre comme un repartitionnement : `Frozen` la fige aussi. Trou connu : supprimer un topic puis le
    recréer autrement ne se voit pas sans historique.
13. **Gardes resserrées** : un topic `per_key` de la base ne change ni d'ordre ni de partitions (sinon on
    contourne en deux PR) ; un seul document YAML ; `service`, `key`, `payload`, `duplicate_unit`
    requis ; `tenant` énuméré, exigé par `per_tenant` et `park_by_tenant` ; une dépendance au moins ;
    `assignment: manual` sur un topic à clé `manual`.

## Design arrêté

- `contracts/topics.yaml` : `topics` (table nom → topic), `groups` (liste). Champs de topic : `key`,
  `order` (`per_key`, `none`), `duplicate_unit`, `payload`, `body` (`none`, `clear_bounded`),
  `partitions`, `retention`, `compacted`, `producers`. Champs de groupe : `topic`, `service`, `lane`,
  `assignment`, `tenant`, `dependencies` (nom → `{mode, justification}`), `concurrency`
  (`per_partition`, `per_tenant`) et `max_in_flight`, `max_age` (`category`, une durée, ou absent),
  `dead_letter`, `start` (`earliest`, `latest`) et `start_justification`. Décodage strict : un champ
  inconnu ou une clé dupliquée est une erreur.
- Le contenu reproduit §9.1 : 14 familles de topics (20 topics avec les catégories), un groupe par
  consommateur de la table, plus la lane de projection (38).
- `order: per_partition` pour une clé `manual` (`mt.routed.<cat>`, « par partition » dans la table de
  §9.1) : ni figé comme `per_key`, ni muet comme `none`. §9.1 nomme la valeur.
- `internal/catalog` :
  - `Parse([]byte) (Catalog, error)` ;
  - `Parse` refuse aussi un second document ;
  - `(Catalog) Validate() []error` : valeurs énumérées (dont `tenant`), champs requis (`service`, `key`,
    `payload`, `duplicate_unit`), une dépendance au moins, `per_tenant` et `park_by_tenant` avec un
    tenant, `assignment: manual` sur une clé `manual`, topic de groupe déclaré, groupe unique,
    `dead_letter` = `<groupe>.dead-letter`, clé `manual` ⇔ `per_partition`, `clear_bounded` ⇒ rétention
    ≤ 96 h, rétention présente
    hors topic compacté, `latest` et `fail_open` justifiés, `max_in_flight` ssi `per_tenant` ;
  - `Frozen(base, head Catalog) []error` : clé, ordre et partitions inchangés des topics `per_key` de la
    base ;
  - `(Catalog) DeadLetters() map[string]Topic` : corps de la source, 8 partitions, 96 h ;
  - `(Catalog) Markdown() []byte` : la page.
- `tools/catalog` : sans option, valide `contracts/topics.yaml` et le compare à `CATALOG_BASE`
  (défaut `origin/main`) ; `-doc <fichier>` écrit la page. `doc.go` racine porte le `go:generate`.
- `docs/topics.md` générée ; `check-generated` la garde.
- `Makefile` : cible `catalog` dans `check`. `ci.yml` : `fetch-depth: 0`.
- Spec 09 §9.1 : champ `assignment`. Design 3 §4.3 : trois groupes `connector-pool-svc.mt.routed.<cat>`.
- Vérification du critère : chaque garde rouge sur une entrée fautive (tests unitaires), le vrai
  catalogue vert, une partition `per_key` modifiée rouge contre la base.
