# Step 004d — Sécurité en CI

Plan : `docs/plan/socle.md` §004d. Mode : PR.

## Contexte

Aucun contrôle de sécurité ne tournait : le `govulncheck` requis et le `gosec` de l'ancienne passerelle
n'avaient pas été repris à la step 002. Lancé sur `main`, `govulncheck` rend déjà 9 vulnérabilités
atteignables de la bibliothèque standard de Go 1.26.7 (`net/http`, `net/textproto`, `crypto/tls`), corrigées
en 1.26.9. `gosec` signale 10 motifs, tous dans `tools/`.

**Terminé quand** : une PR qui ajoute une dépendance vulnérable connue, ou un motif signalé par `gosec`, est
rouge ; CodeQL publie ses résultats sur la PR ; Dependabot a ouvert sa première PR.

## Arbitrages

Le plan tranche le périmètre ; ce qui reste est de la forme, sans choix produit :

- **Go 1.26.9** : la directive `go` monte, la CI suit (`go-version-file`).
- **`gosec`** : les permissions des fichiers écrits par les outils et leurs tests descendent à `0o600` et
  `0o750` (corriger plutôt qu'exclure). Deux exclusions, sous `tools/` seulement, chacune nommant ses règles
  et sa raison : G304 et G703, sa variante par propagation (les outils lisent et écrivent des chemins du
  dépôt, aucune entrée non fiable) ; G204 et G702, sa variante par propagation (`git` lancé avec une liste
  d'arguments, sans shell, sur une référence fournie par le développeur ou la CI).
- **CodeQL** : un job `codeql` du workflow unique, Go en `autobuild`, sur `push` et `pull_request` seulement ;
  `security-events: write` au job, le reste du workflow garde `contents: read`. Informatif : hors des
  `needs` de l'agrégateur `ci`. Le rendre requis après le tri des premiers résultats est une dette, ouverte
  dans la PR (#88), marquée dans le workflow.
- **Dependabot** : hebdomadaire, modules Go et actions ; un groupe par écosystème, pour une PR à la fois ;
  `allow: all` pour les modules, puisque les outils (`tool`) ne sont requis qu'en indirect ; titres
  `build(deps): …` et `ci(deps): …`, que la garde de titre accepte.
- Les actions de CodeQL sont épinglées par SHA (v4.38.3), comme les autres.

## Design arrêté

- `go.mod` : `go 1.26.9` ; `golang.org/x/vuln/cmd/govulncheck` v1.9.0 par la directive `tool`.
- `Makefile` : cible `vuln` (`go tool govulncheck ./...`), dans `check`.
- `.golangci.yml` : `gosec` activé ; deux exclusions commentées sous `tools/`.
- `tools/catalog`, `tools/openapi` : permissions resserrées.
- `.github/workflows/ci.yml` : job `codeql` (`init`, `analyze`), hors de l'agrégateur, `debt: #88`.
- `.github/dependabot.yml`.
- Vérification du critère : sur une copie jetable, une dépendance vulnérable appelée (`golang.org/x/text`
  v0.3.7, `language.Parse`, GO-2022-1059) fait échouer `make vuln`, un `exec.Command` sur une variable
  hors de `tools/` fait échouer `make lint` ; CodeQL publie sur la PR ; la première PR de Dependabot suit le
  merge.
