# Step 002 — Squelette Go et CI

Plan : `docs/plan/socle.md` §002. Décisions : C1, C3, C9 (`docs/design/09-conventions-du-socle.md`).
Mode : push direct, le dernier ; la protection de `main` le clôt.

## Contexte

Le dépôt n'a encore aucun code Go. Cette step pose le module, la commande qui fait foi (`make check`) et
le workflow unique, puis ferme `main` aux pushs directs.

**Terminé quand** : une PR qui ajoute un test sauté, un symbole exporté sans doc ou un fichier non
formaté est rouge ; la même PR corrigée est verte.

## Arbitrages

1. **`ciguard` se place sur la sortie de `go test`, pas dans une bibliothèque.** L'ancien `ciguard`
   (go-gateway) est une fonction que le test appelle à la place de `t.Skip` : un `t.Skip` direct lui
   échappe, ce que le critère de fin interdit. La garde lit donc la sortie de `go test -v` et échoue sur
   toute ligne `--- SKIP` ; un log qui contiendrait ce texte la ferait échouer, jamais passer. Elle
   couvre aussi les helpers d'intégration à venir, qui peuvent sauter librement hors `make check`.
   L'ancienne bibliothèque n'est pas portée. Tranché par la spec (critère de fin, C3).
2. **Les cinq outils entrent dès maintenant** par la directive `tool` (golangci-lint, sqlc, buf,
   oapi-codegen, goose), comme le plan le dit : un conflit de versions entre leurs dépendances, dans un
   seul `go.mod` (C1, « une version par dépendance »), se découvre ici une fois pour toutes. La version
   de golangci-lint n'est écrite qu'à un endroit, `go.mod`.
3. **`gofmt` passe par golangci-lint** (`formatters`), sans second outil ; `simplify: false` l'accorde
   à `gofmt -l` et aux éditeurs. `goimports` et `gofumpt` n'entrent pas (C9).
4. **Linters** : le jeu par défaut de golangci-lint v2, plus `revive` réduit à la règle `exported`.
   Aucun preset d'exclusion : le preset `comments` désactiverait justement `exported`.
5. **Titre de PR conventionnel** : une regex dans un job du workflow, sans action tierce.
6. **`check-generated`** : l'état de l'arbre avant et après `go generate ./...` doit être identique,
   pour fonctionner aussi sur un arbre local non commité. `git status` liste les fichiers non suivis un
   par un (`-uall`) : sans cela, un fichier généré dans un dossier non suivi passe inaperçu. Aucun
   générateur aujourd'hui ; les steps 004 et 006 en ajoutent.
7. **`doc.go` racine** : golangci-lint refuse un module sans fichier Go ; le paquet racine ne porte que
   sa doc.
8. **Protection de `main`** : un ruleset GitHub, PR obligatoire, check `ci` requis et émis par GitHub
   Actions, aucun contournement. Merge par squash seulement : le titre de la PR devient le commit de
   `main`, ce qui donne son sens à la regex. Suppression et push forcé interdits. Aucune approbation
   exigée : l'humain merge lui-même.

## Design arrêté

- `go.mod` : `github.com/martialanouman/sms-platform`, version de Go locale, bloc `tool` des cinq
  outils ; `doc.go` racine.
- `.golangci.yml` (v2) : linters par défaut + `revive` (`exported`) ; formatter `gofmt`.
- `Makefile` :
  - `help` : liste les cibles commentées `##` ;
  - `lint` : `go tool golangci-lint run` ;
  - `test` : `go test -race ./...` (les sauts sont permis) ;
  - `ciguard` : `go test -race -v ./...`, échec sur une ligne `--- SKIP` (`pipefail` garde l'échec
    des tests) ;
  - `check-generated` : `git status --porcelain -uall` et `git diff` identiques avant et après
    `go generate ./...` ;
  - `check` : `lint ciguard check-generated`, ce que vérifie la CI.
- `.github/workflows/ci.yml`, workflow unique (`push` sur `main`, `pull_request`) :
  - `check` : checkout + setup-go épinglés par SHA, `make check` ;
  - `pr-title` (PR seulement) : regex conventionnelle sur le titre ;
  - `ci` : agrégateur, `needs` des deux, échoue si l'un a échoué ou été annulé ; seul check requis.
- Ruleset `main` : PR obligatoire (squash seul), `ci` vert émis par GitHub Actions, ni suppression ni
  push forcé, pas de bypass.
- Vérification du critère : une PR de démonstration (test sauté, symbole exporté sans doc, fichier non
  formaté), rouge, puis corrigée, verte ; fermée sans merge.
