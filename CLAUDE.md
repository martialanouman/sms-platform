# CLAUDE.md — Plateforme SMS (passerelle et tableau de bord)

Ce que le dépôt ne dit pas de lui-même : ce qu'on construit, les invariants, l'ordre qu'on ne devine
pas, les couplages invisibles. Les règles de `.claude/rules/` se chargent seules à la lecture d'un
fichier de leur territoire ; ce qui doit se savoir **avant** d'ouvrir un fichier est ici.

## Ce qu'on construit

Une **passerelle SMS** d'agrégateur national (SMPP et REST en entrée, SMPP et HTTP en sortie, voie
retour MO et DLR), à **8 000 SMS/s** soutenus, et son **tableau de bord d'exploitation** (BFF Go, SPA
React). Pas une plateforme de campagnes. Mono-pays.

La spec fait foi : `docs/spec/` (point d'entrée `docs/spec/README.md`). Les décisions qui la fondent :
`docs/design/`. Les ADR : `docs/adr/`. Le plan en cours : `docs/plan/`.

## Ordre du pipeline MT — non réordonnable

auth → débit (sender ID puis compte) → choix du topic et quiet hours → **ACK durable** → E.164 →
autorisation du sender ID → opt-out → anti-spam → route (numéro exact → script → déclaratif) et choix
du compte opérateur → encodage et segmentation → crédit → débit du compte opérateur et jeton
d'annulation → envoi → règlement → état et CDR.

Le débit se refuse **avant** l'ACK, jamais après. **I3** — un message routé par numéro exact
traverse toutes les étapes de conformité (sender ID, opt-out, anti-spam) : le court-circuit saute la
résolution de route, **jamais** la conformité. Détail : `docs/spec/03-pipeline-mt.md`.

## Invariants

**I1 à I15**, numérotés dans `docs/spec/README.md`. Le code les cite par leur numéro ; chacun a un test
bloquant, vert à vie. Les trois qu'on casse sans s'en rendre compte :

- **I1** — le corps d'un message ne sort jamais : ni log, ni span, ni label, ni erreur, ni DTO non
  déclaré.
- **I7** — un seul écrivain de l'état du message, `message-state-svc`.
- **I13** — toute écriture du plan de contrôle pose `app.actor`, sinon elle échoue.

## Les couplages qu'on oublie

Une règle ne se charge qu'à la **lecture** d'un fichier de son territoire : créer un fichier neuf n'en
déclenche aucune. D'où ces déclencheurs :

- **Ajouter un code d'erreur** → catalogue Go, `enum` des contrats OpenAPI, table de
  `docs/spec/09-contrats.md` §9.3, ensemble.
- **Toucher un contrat** (`contracts/**`) → le contrat **avant** l'implémentation ; régénérer.
- **Ajouter ou consommer un topic** → `contracts/topics.yaml` d'abord ; le runtime refuse un topic non
  déclaré.
- **Ajouter une table** → une migration goose, et son classement d'audit, et la déclaration de GRANT
  des services qui la lisent.
- **Différer quelque chose** (« hors périmètre », « pour l'instant », « deferred ») → une issue
  étiquetée `debt` **dans la même PR**, et `debt: #N` là où le code porte le report.
- **Ajouter un `cmd/`** → GoReleaser et `deploy/k8s/`, dans la même PR.

## Travailler ici

- **Une step = une PR**, numérotée dans l'ordre d'exécution (`docs/plan/`). Son design s'écrit dans
  `docs/steps/NNN-slug.md`, sous `## Design arrêté`, et se commite **avant** la première ligne de code.
- **TDD strict**, mutation obligatoire : un test qu'on n'a jamais vu échouer n'est pas un test.
- **Arbitrage** : la spec d'abord ; le modèle Fable ensuite ; l'humain en dernier, avec une
  recommandation. Jamais un choix silencieux.
- **Le merge est à l'humain.** Titre de PR conventionnel.
- **Le dépôt est public** jusqu'à la fin du socle et des composants : aucun secret, aucune donnée
  réelle, jamais.

## Code

- Code en anglais ; interface, docs et messages de commit en français.
- **Zéro commentaire par défaut.** Un commentaire ne survit que s'il porte un pourquoi qu'on ne peut
  pas déduire du code et dont l'ignorance coûte cher. Sauf ce qu'exige `revive exported` : une ligne de
  doc par symbole exporté.
- Bibliothèques : la doc par `ctx7` avant d'écrire, jamais de mémoire.

`make help` fait foi pour les commandes ; `make check` agrège ce que vérifie la CI.
