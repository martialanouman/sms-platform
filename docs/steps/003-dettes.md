# Step 003 — Dettes

Plan : `docs/plan/socle.md` §003. Décisions : C5 et §3 de `docs/design/09-conventions-du-socle.md`.
Mode : PR.

## Contexte

Les dettes deviennent des issues GitHub étiquetées `debt`, et le code qui porte un report le marque
`debt: #N`. Cette step pose les étiquettes, le gabarit, la garde de CI, et trie les registres des deux
anciens dépôts. C'est aussi le premier code Go du dépôt : il applique la clean architecture, que
l'humain impose le 2026-10-17 et que l'ADR 0025 écrit.

**Terminé quand** : la garde est rouge sur une marque orpheline, et chaque fiche a un sort écrit.

## Arbitrages

1. **Clean architecture, forme « cœur + adaptateurs »** (choix de l'humain, recommandé). Par contexte
   métier `internal/<ctx>/` : un paquet cœur (entités, règles, cas d'usage, ports) et un sous-paquet par
   adaptateur `internal/<ctx>/<techno>/`. Les racines de composition sont `cmd/<service>/` (déployé) et
   `tools/<outil>/` (outillage de CI, jamais déployé, hors GoReleaser). L'outillage partagé vit sous
   `internal/platform/<lib>/`. La spec ne disait rien ; aucune contradiction.
2. **La règle de dépendance se garde par depguard, sur l'I/O seulement.** Un adaptateur importe les
   types de son cœur ; le cycle étant interdit par Go, le cœur ne peut pas importer son adaptateur. La
   garde interdit donc au cœur (fichiers `internal/<ctx>/*.go` hors tests) toute bibliothèque d'I/O :
   liste blanche, `$gostd` et le module seulement, et dans la bibliothèque standard `net`, `os`,
   `database/sql`, `syscall` refusés. Une bibliothèque tierce pure (uuid…) s'ajoute à la liste blanche
   quand un cœur en a besoin. Trou connu : un cœur qui importerait l'adaptateur d'un **autre** contexte ;
   la revue le voit, aucune garde ne le voit.
3. **La garde des dettes est un outil Go**, pas une ligne de shell : le cœur `internal/debt` (lire les
   marques d'un `fs.FS`, calculer les orphelines) se teste sans réseau ; l'adaptateur
   `internal/debt/github` parle à l'API REST d'issues (pagination, exclusion des PR) ; `tools/debtguard`
   compose. Une marque est `debt: #` suivi de chiffres, dans n'importe quel fichier du dépôt hors
   `.git`.
4. **Jeton** : facultatif (dépôt public). En CI, `GH_TOKEN` vient de `github.token` avec
   `issues: read` ; en local, `make` le prend à `gh auth token` s'il existe.
5. **Fermer l'issue fait échouer la CI** : le workflow se déclenche aussi sur `issues` (`closed`,
   `unlabeled`, `deleted`) et rejoue `make check` sur `main` ; toute PR ouverte, qui porte la marque,
   est rouge elle aussi. La liste filtrée par étiquette a environ deux secondes de retard sur une
   fermeture (mesuré) ; le run déclenché démarre bien après, et un vert périmé ne survit pas à la PR
   suivante.
6. **Gabarit** : un formulaire d'issue `.github/ISSUE_TEMPLATE/debt.yml`, cinq champs obligatoires,
   étiquette `debt` posée d'office. Les étiquettes `debt` et `go-live` se créent par `gh label create`.
7. **Le plan se trompe de compte** : le registre de go-gateway a 108 fiches, dont 15 `PAYÉE` ; 93 sont
   ouvertes. Celui de go-gateway-bo a 29 fiches et un `README.md`. 122 fiches à trier, pas 138. Le plan
   est corrigé dans cette PR.
8. **Reprise** : trié par lots en lecture seule ; chaque fiche reçoit `soldee` (décision et section
   citées), `disparait` (dashboard, liée aux relais) ou `issue`. Les verdicts `soldee` et `disparait`
   sont listés dans la description de la PR ; les issues sont créées avant la PR, pour qu'elle les cite.
9. **Le trou des build tags** laissé par la step 002 est un report : il devient une issue `debt`, et sa
   marque se pose sur la cible `ciguard`.

## Design arrêté

- `docs/adr/0025-clean-architecture-coeur-et-adaptateurs.md`, ligne à l'index ; `CLAUDE.md` (section
  Code) et `.claude/rules/go-code.md` disent la forme.
- Une marque s'écrit `debt:`, des espaces facultatifs, `#` et des chiffres : une marque tapée sans espace
  ne doit pas échapper à la garde (mutation survivante).
- `.golangci.yml` : `depguard`, une règle `core` (fichiers `**/internal/*/*.go`, `!$test`).
- `internal/debt` :
  - `Mark{Path, Line, Issue}` ;
  - `Scan(fs.FS) ([]Mark, error)` ;
  - port `Issues` : `OpenDebt(ctx) (map[int]bool, error)` ;
  - `Orphans(ctx, fs.FS, Issues) ([]Mark, error)`.
- `internal/debt/github` : `Client{BaseURL, Repo, Token, HTTP}` implémente `Issues` par
  `GET /repos/{repo}/issues?labels=debt&state=open&per_page=100&page=N`, jusqu'à une page vide ; les
  entrées portant `pull_request` sont ignorées ; un statut ≠ 200 est une erreur.
- `tools/debtguard/main.go` : lit `GITHUB_REPOSITORY`, `GH_TOKEN` ; scanne `.` ; imprime
  `chemin:ligne: debt: #N ne désigne aucune issue ouverte étiquetée debt` et sort en 1.
- `Makefile` : cible `debtguard`, entrée dans `check`.
- `ci.yml` : `issues` dans `on`, `permissions: issues: read`, `GH_TOKEN` pour `check`.
- `.github/ISSUE_TEMPLATE/debt.yml`.
- Plan §003 corrigé (122 fiches).
- Vérification du critère : une marque vers une issue inexistante, puis vers une issue fermée, rend
  `make check` rouge ; retirée, vert. Idem en CI sur la PR.
