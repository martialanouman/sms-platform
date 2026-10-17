# ADR-0025 : Clean architecture, un cœur et ses adaptateurs par contexte

**Status:** Accepted
**Date:** 2026-10-17
**Deciders:** Demande humaine du 17/10/2026 (step 003) ; forme choisie par l'humain sur recommandation.
**Réf spec:** aucune ; `docs/design/09-conventions-du-socle.md` (C1) ; `docs/steps/003-dettes.md`

## Context

Le monorepo n'a encore aucun code métier, et treize services vont l'habiter. Les anciens dépôts n'avaient
qu'une règle : le métier sous `internal/`, `cmd/` qui câble, les interfaces côté consommateur. Rien
n'empêchait un paquet métier d'ouvrir une connexion, de lire l'environnement ou d'importer un pilote.
L'humain impose la clean architecture au dépôt entier.

## Decision

**1. Un contexte métier est un paquet cœur et des adaptateurs.**
- `internal/<ctx>/` : entités, règles, cas d'usage, et les ports qu'il consomme (interfaces).
- `internal/<ctx>/<techno>/` : un adaptateur par technologie (`postgres`, `kafka`, `github`…) qui
  implémente un port du cœur.
- Pas de couches `domain/`, `app/`, `adapter/` distinctes : deux niveaux suffisent à la règle de
  dépendance, sans paquet de cérémonie pour un petit contexte.

**2. Les racines de composition** sont `cmd/<service>/` (déployé) et `tools/<outil>/` (outillage de CI,
jamais déployé, hors GoReleaser et `deploy/k8s/`). Elles construisent les adaptateurs et les passent au
cœur. L'outillage partagé (step 005) vit sous `internal/platform/<lib>/`.

**3. La règle de dépendance pointe vers le cœur.** Un adaptateur importe son cœur ; comme Go interdit
les cycles, le cœur ne peut pas importer son adaptateur. depguard (règle `core` de `.golangci.yml`)
garde le reste : un fichier `internal/<ctx>/*.go` hors tests n'importe que la bibliothèque standard et le
module, et dans la bibliothèque standard ni `net`, `net/http`, `net/rpc`, `net/smtp`, `os`,
`database/sql`, `syscall`, `io/ioutil`, `crypto/tls`, `log`, ni `log/syslog`.
`log/slog` reste permis : son handler est l'adaptateur. `path/filepath` et `crypto/rand` aussi, pour
leurs fonctions de calcul ; un cœur qui lit le disque passe par `fs.FS`. Une bibliothèque tierce pure
s'ajoute à la liste blanche quand un cœur en a besoin.

## Consequences

- Un cœur se teste sans réseau, sans base, sans environnement : ses ports se doublent par des types de
  test, ses fichiers par `fs.FS`.
- Trou connu : un cœur qui importerait l'adaptateur d'un **autre** contexte. Aucune garde ne le voit ;
  la revue, si.
