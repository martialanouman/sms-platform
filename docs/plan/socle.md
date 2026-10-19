# Plan du socle

Ce qui doit exister avant le premier composant : la décision 9 appliquée, plus l'outillage partagé que
tous les services importeront. Le numéro est l'ordre d'exécution (C6). Aucun service n'est écrit ici.

| Step | Livre | Mode |
|---|---|---|
| 001 | Contexte de travail : `CLAUDE.md`, règles, mémoire, ADR | Push direct |
| 002 | Squelette Go et CI unique ; protection de `main` | Push direct, le dernier |
| 003 | Dettes en issues et leur garde | PR |
| 004a | Contrats : catalogue des topics | PR |
| 004b | Contrats : protos | PR |
| 004c-1 | Contrats : OpenAPI public, `oasdiff` | PR |
| 004c-2 | Contrats : OpenAPI Admin, scopes générés, gardes | PR |
| 004c-3 | Contrats : OpenAPI BFF, permissions | PR |
| 004d | Sécurité en CI | PR |
| 005a | Bibliothèques : erreurs, corps | PR |
| 005b | Bibliothèques : observabilité | PR |
| 005c | Bibliothèques : mTLS | PR |
| 005d | Bibliothèques : configuration d'infrastructure, drain | PR |
| 005e | Bibliothèques : tests | PR |
| 006a | Données : goose, `control_plane`, trigger d'audit, classement | PR |
| 006b | Données : rôles, GRANT générés | PR |
| 007a | Livraison : images, scan, `migrate`, squelette `deploy/k8s/` | PR |
| 007b-1 | Livraison : `catalog-apply`, topics et ACL Kafka | PR |
| 007b-2 | Livraison : `catalog-apply`, GRANT en production | PR |
| 007c | Livraison : NetworkPolicy générées, alertes | PR |
| 007d-1 | Livraison : briques de l'environnement de test | PR |
| 007d-2a | Livraison : tag et release | PR |
| 007d-2b | Livraison : déploiement par tag, retour arrière | PR |

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
- **Reprise** : les 122 fiches ouvertes des deux registres (93 et 29) sont triées. Une fiche soldée par
  une décision est listée avec sa décision dans la description de la PR ; une autre devient une issue.
  Les dettes de `go-gateway-bo` liées aux 33 relais disparaissent avec eux.

**Terminé quand** : la garde est rouge sur une marque orpheline, et chaque fiche a un sort écrit.

## 004 — Contrats

Découpée en 004a (catalogue), 004b (protos) et 004c (OpenAPI) par décision humaine du 2026-10-17 :
la step entière dépassait une PR relisible. Le critère de fin se partage entre elles. La 004c est à son tour
découpée en trois PR, une par contrat (décision humaine du 2026-10-18).


- **`contracts/topics.yaml`** et son chargeur typé : tous les topics et groupes du chapitre 9 §9.1.
  Gardes statiques dès maintenant : `dead_letter` obligatoire, corps `clear_bounded` ≤ 96 h,
  partitions figées d'un topic `per_key`, `latest` justifié, `fail_open` justifié. Les gardes qui lisent
  le code (1 à 3, 6) arrivent avec le runtime de lane. Page de documentation générée.
- **`contracts/proto/`** : buf, `lint` et `breaking` bloquants ; les payloads des topics et le gRPC du
  chapitre 9 §9.2.
- **OpenAPI** public, Admin et BFF, repris des anciens dépôts et **mis directement à l'état des
  décisions** (scopes, codes d'erreur, `charge_on`, champs retirés), en version 1.0.0 : aucune version
  n'a été mise en production, il n'y a donc aucun client à ménager. Liste des scopes générée.
- `check-generated` couvre la liste générée.

**Terminé quand** : le catalogue reproduit le chapitre 9 ; chaque garde statique est rouge sur une
entrée fautive ; `oasdiff` et `buf breaking` tournent sur la PR.

## 004d — Sécurité en CI

Ajoutée le 2026-10-17 par décision humaine : aucun contrôle de sécurité n'était prévu, et le
`govulncheck` requis et le `gosec` de l'ancienne passerelle n'ont pas été repris à la step 002.

- **`govulncheck`** par la directive `tool`, bloquant dans `make check`.
- **`gosec`** dans golangci-lint ; une exclusion nomme sa règle et dit pourquoi.
- **CodeQL** (Go) : un job du workflow unique (C3), résultats dans le code scanning. Informatif au
  départ, requis une fois les premiers résultats triés.
- **Dependabot** pour les modules Go et les actions épinglées par SHA ; titres de PR conventionnels.

**Terminé quand** : une PR qui ajoute une dépendance vulnérable connue, ou un motif signalé par `gosec`,
est rouge ; CodeQL publie ses résultats sur la PR ; Dependabot a ouvert sa première PR.

## 005 — Bibliothèques de plateforme

Découpée en cinq PR par décision humaine du 2026-10-18 : 005a (erreurs, corps), 005b (observabilité),
005c (mTLS), 005d (configuration d'infrastructure, drain), 005e (tests). Chacune porte le critère de fin
pour ses bibliothèques.

Portées de `go-gateway` avec leurs tests, puis adaptées aux décisions :

- **Erreurs** : le catalogue du chapitre 9 §9.3, mapping REST et SMPP, test contre les `enum` des
  contrats.
- **Corps** : le type qui ne se sérialise pas, `Reveal` audité (I1), et sa garde.
- **Observabilité** : logger structuré, garde des labels de métriques, traces OpenTelemetry avec
  échantillonnage des échecs.
- **mTLS** : serveur et client depuis la CA interne ; allowlist obligatoire, vide = échec
  du démarrage (décision 6).
- **Configuration d'infrastructure** : chargeur d'environnement limité à l'infrastructure, garde AST
  contre toute configuration métier (décision 7).
- **Drain** : `OnDrain`, `/readyz`, ordre des composants.
- **Tests** : testcontainers (socket exporté, Ryuk actif) et `tcpproxy` ; les `Cuttable` par magasin
  arrivent avec leur premier consommateur (pour Postgres, la bibliothèque d'instantané de
  configuration ; amendé en 006a).

**Terminé quand** : chaque bibliothèque a son test, et chaque garde a été prouvée par une mutation.

## 006 — Données

Deux PR (Fable, 2026-10-18) : 006a pour goose, l'audit et le classement ; 006b pour les rôles et les
GRANT, qui lisent le classement.

- **goose** en bibliothèque, un répertoire par base. Seule `control_plane` naît ici ; `message_state`,
  `dashboard` et `cdr` (avec le câblage goose de ClickHouse) arrivent avec leur premier composant ; avec
  la deuxième base, `CONNECT` est révoqué de `PUBLIC` sur chaque base, `control_plane` comprise, et
  accordé aux seuls rôles qui y sont déclarés (007b-2).
  up/down/up prouvé en CI par un test d'intégration, sous `migrator` ; le Job `migrate` arrive en 007.
- **Rôles** : `migrator` propriétaire des schémas ; un rôle par service ; GRANT **générés** depuis un
  fichier de déclaration par service (décision 6). Une garde refuse un droit non déclaré.
- **Audit et notification** : `audit_log`, `audit_changes`, la fonction de trigger (acteur obligatoire,
  avant/après, colonnes scellées exclues, `pg_notify`), le classement des tables et sa garde. Aucune
  table métier encore : chacune arrivera avec son composant et devra se classer ; le trigger par
  instruction (`<table>:*`) et le canal `optout` arrivent avec `exact_routes` et `suppressions`.

**Terminé quand** : une écriture sans `app.actor` échoue ; une table non classée fait échouer la garde ;
un rôle de service ne lit rien hors de sa déclaration (testé sous un rôle non superuser).

## 007 — Livraison

Sept PR (Fable, 2026-10-19 ; 007d-2 coupée en deux par l'humain) ; chacune porte sa part du critère de fin,
atteint en 007d-2b. Décisions
humaines du 2026-10-19 : NetworkPolicy générées, jamais appliquées par `catalog-apply` ; environnement de
test tiré par la VM ; déploiement par tag de release, pas par SHA (décision du même jour).

- **007a** : **scan des images** produites (vulnérabilités connues), bloquant (ajouté avec la step 004d) ;
  **GoReleaser**, une version, images distroless ; la garde : chaque `cmd/` a exactement un build, une
  image et un manifest (13 à terme) ; `cmd/migrate` ; le squelette **`deploy/k8s/`**, sa garde et le Job `migrate`.
- **007b-1** : `cmd/catalog-apply`, topics et ACL Kafka depuis le catalogue ; son Job.
- **007b-2** : **GRANT en production** (reports de 006b) : `catalog-apply` applique
  `migrations/grants/control_plane.sql` sous `migrator`, une seule fois à la fois, en échouant sur la
  première erreur ; il pose `LOGIN` et le secret de chaque rôle de service, et retire les droits d'un rôle
  sorti des déclarations, que le fichier généré ne nomme plus.
- **007c** : NetworkPolicy **générées** dans `deploy/k8s/` depuis les déclarations, sous
  `check-generated` ; **un flux entre avec le workload qui le déclare** (allowlists et port ops avec le
  premier service) ; **`deploy/alerts/`** : règles Prometheus, `promtool test rules` en CI. Seules
  entrent les règles dont la métrique source existe ; **une règle entre avec le composant qui émet sa
  métrique**, le tableau de la spec 10 §10.1 sert de liste de contrôle.
- **007d-1** : les briques de l'environnement de test (opérateur Postgres, Strimzi, cert-manager),
  versions épinglées, TLS vers Postgres ; le Secret `postgres-password` de chaque service déclaré (32
  caractères ASCII imprimables au moins, tirés au hasard) et le Secret `catalog-apply` (URL sous
  `migrator`), avant tout service. `breakglass` et le rôle `CREATEROLE` d'OpenBao entrent avec la brique
  OpenBao, par les rôles gérés de CloudNativePG, sous un autre rôle que `migrator`, qui ne crée que les rôles
  de service (007b-2, 007d-1).
- **007d-2a** : **tag et release** : un merge dont les commits changent le binaire (`feat` → mineure ; `fix`,
  `perf`, `refactor`, `build`, `revert` → patch ; rupture → majeure, mineure en 0.x) pose le tag `vX.Y.Z`,
  publie la release et pousse les images `:vX.Y.Z` sur GHCR avec le `GITHUB_TOKEN`.
- **007d-2b** : **environnement de test** k3s : la VM tire le dernier tag de release et déploie ses images ;
  chaque Job est borné dans le temps ; aucun secret de déploiement dans GitHub ; retour arrière par `git revert`,
  qui produit une release.

**Terminé quand** : une release déploie l'environnement de test sans geste manuel, et `catalog-apply` y crée les
topics et leurs ACL.

## Après le socle

Les composants : `docs/plan/composants.md`. Ordre esquissé à l'origine, dans l'ordre des dépendances : runtime de lane et instantané de
configuration (partagés), `content-key-svc`, `admin-api-svc` et le dashboard, puis le chemin MT
(ingestion, routeur, `billing-svc`, pool), `message-state-svc`, la voie retour. Chaque composant est
porté ou réécrit selon ce que la spec en exige.
