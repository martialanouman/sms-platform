# Plan du socle

Ce qui doit exister avant le premier composant : la décision 9 appliquée, plus l'outillage partagé que
tous les services importeront. Le numéro est l'ordre d'exécution (C6). Aucun service n'est écrit ici.

| Step | Livre | Mode |
|---|---|---|
| 001 | Contexte de travail : `CLAUDE.md`, règles, mémoire, ADR | Push direct |
| 002 | Squelette Go et CI unique ; protection de `main` | Push direct, le dernier |
| 003 | Dettes en issues et leur garde | PR |
| 004 | Contrats : catalogue des topics, protos, OpenAPI | PR |
| 005 | Bibliothèques de plateforme | PR |
| 006 | Données : migrations, rôles, GRANT, trigger d'audit | PR |
| 007 | Livraison : images, manifests, Jobs, environnement de test, alertes | PR |

## 001 — Contexte de travail

Objectif : qu'une session ouverte dans ce dépôt sache ce qu'elle doit savoir, sans les anciens dépôts.

- **`CLAUDE.md`** : ce qu'on construit, l'ordre du pipeline, renvoi aux invariants I1–I15, les
  couplages qu'on oublie (code d'erreur, contrat, schéma, dette, topic). Rien de ce que le dépôt dit
  de lui-même.
- **`.claude/rules/`**, une règle par territoire : `go-code`, `tests` (discipline de mutation, pièges
  des tests creux), `contracts`, `errors`, `migrations`, `topics`, `ci-deploy`, `steps` (dont les
  dettes : issues `debt`, marque `debt: #N`). Les enseignements de l'annexe de l'audit y entrent comme règles, dans le
  territoire où ils mordent.
- **Mémoire neuve** du projet `sms-platform` : seulement les mémoires de méthode (TDD strict, mutation,
  revue, merge par l'humain, recommandation plutôt que QCM). L'historique des steps reste dans les
  anciens dépôts (C10).
- **ADR** : `docs/adr/` reçoit 0001 à 0024 tels quels, chacun précédé d'un bandeau « Amendé par » qui
  renvoie à la décision et à la section concernées. La série continue à 0025 (C7).

**Terminé quand** : une session neuve, ouverte dans `~/Sources/sms-platform`, cite I3 et l'ordre du
pipeline sans lire la spec.

## 002 — Squelette Go et CI

- **Un `go.mod`** (`github.com/martialanouman/sms-platform`) ; outils par la directive `tool` :
  golangci-lint, sqlc, buf, oapi-codegen, goose (C1, C9).
- **`make check`** agrège ce que vérifie la CI ; `make help`.
- **Un workflow** : un job agrégateur requis, actions épinglées par SHA, `gofmt`, golangci-lint
  (`revive exported`), `go test -race`, `ciguard` (un test sauté échoue), `check-generated` (vide pour
  l'instant). Titre de PR conventionnel.
- **Protection de `main`** : PR obligatoire, agrégateur vert. Plus aucun push direct ensuite.

**Terminé quand** : une PR qui ajoute un test sauté, un symbole exporté sans doc ou un fichier non
formaté est rouge ; la même PR corrigée est verte.

## 003 — Dettes

- Étiquettes `debt` et `go-live` ; gabarit d'issue aux champs obligatoires (ce qu'on a fait à la place,
  pourquoi, coût, déclencheur, porteur).
- **Garde** : chaque `debt: #N` du dépôt désigne une issue ouverte étiquetée `debt`. Fermer l'issue sans
  retirer la marque fait échouer la CI.
- **Reprise** : les 138 fiches ouvertes des deux registres (108 et 30) sont triées. Une fiche soldée par
  une décision est listée avec sa décision dans la description de la PR ; une autre devient une issue.
  Les dettes de `go-gateway-bo` liées aux 33 relais disparaissent avec eux.

**Terminé quand** : la garde est rouge sur une marque orpheline, et chaque fiche a un sort écrit.

## 004 — Contrats

- **`contracts/topics.yaml`** et son chargeur typé : tous les topics et groupes du chapitre 9 §9.1.
  Gardes statiques dès maintenant : `dead_letter` obligatoire, corps `clear_bounded` ≤ 96 h,
  partitions figées d'un topic `per_key`, `latest` justifié, `fail_open` justifié. Les gardes qui lisent
  le code (1 à 3, 6) arrivent avec le runtime de lane. Page de documentation générée.
- **`contracts/proto/`** : buf, `lint` et `breaking` bloquants ; les payloads des topics et le gRPC du
  chapitre 9 §9.2.
- **OpenAPI** public, Admin et BFF, repris des anciens dépôts et **mis directement à l'état des
  décisions** (scopes, codes d'erreur, `charge_on`, champs retirés), en version 1.0.0 : aucune version
  n'a été mise en production, il n'y a donc aucun client à ménager. Liste des scopes générée.
- `check-generated` couvre les types générés.

**Terminé quand** : le catalogue reproduit le chapitre 9 ; chaque garde statique est rouge sur une
entrée fautive ; `oasdiff` et `buf breaking` tournent sur la PR.

## 005 — Bibliothèques de plateforme

Portées de `go-gateway` avec leurs tests, puis adaptées aux décisions :

- **Erreurs** : le catalogue du chapitre 9 §9.3, mapping REST et SMPP, test contre les `enum` des
  contrats.
- **Corps** : le type qui ne se sérialise pas, `Reveal` audité (I1), et sa garde.
- **Observabilité** : logger structuré, garde des labels de métriques, traces OpenTelemetry avec
  échantillonnage des échecs.
- **mTLS** : serveur et client depuis la CA interne ; allowlist obligatoire, vide en production = échec
  du démarrage (décision 6).
- **Configuration d'infrastructure** : chargeur d'environnement limité à l'infrastructure, garde AST
  contre toute configuration métier (décision 7).
- **Drain** : `OnDrain`, `/readyz`, ordre des composants.
- **Tests** : testcontainers (socket exporté, Ryuk actif), `Cuttable` et `tcpproxy`.

**Terminé quand** : chaque bibliothèque a son test, et chaque garde a été prouvée par une mutation.

## 006 — Données

- **goose** pour Postgres et ClickHouse, un répertoire par base : `control_plane`, `message_state`,
  `dashboard`, `cdr`. Job `migrate` up/down/up en CI.
- **Rôles** : `migrator` propriétaire des schémas ; un rôle par service ; GRANT **générés** depuis un
  fichier de déclaration par service (décision 6). Une garde refuse un droit non déclaré.
- **Audit et notification** : `audit_log`, `audit_changes`, la fonction de trigger (acteur obligatoire,
  avant/après, colonnes scellées exclues, `pg_notify`), le classement des tables et sa garde. Aucune
  table métier encore : chacune arrivera avec son composant et devra se classer.

**Terminé quand** : une écriture sans `app.actor` échoue ; une table non classée fait échouer la garde ;
un rôle de service ne lit rien hors de sa déclaration (testé sous un rôle non superuser).

## 007 — Livraison

- **GoReleaser** : une version, images distroless ; la garde « exactement N images » suit le nombre de
  `cmd/` (13 à terme).
- **`deploy/k8s/`** et sa garde ; Jobs `migrate` et `catalog-apply` (topics, ACL Kafka, GRANT,
  NetworkPolicy depuis les déclarations).
- **`deploy/alerts/`** : règles Prometheus, `promtool test rules` en CI.
- **Environnement de test** k3s, déployé par SHA depuis `main`, avec retour arrière.

**Terminé quand** : un merge sur `main` déploie l'environnement de test sans geste manuel, et `catalog-apply`
y crée les topics et leurs ACL.

## Après le socle

Les composants, dans un second plan, dans l'ordre des dépendances : runtime de lane et instantané de
configuration (partagés), `content-key-svc`, `admin-api-svc` et le dashboard, puis le chemin MT
(ingestion, routeur, `billing-svc`, pool), `message-state-svc`, la voie retour. Chaque composant est
porté ou réécrit selon ce que la spec en exige.
