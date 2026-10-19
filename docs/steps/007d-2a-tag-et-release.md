# Step 007d-2a — Livraison : tag et release

Plan : `docs/plan/socle.md` §007, sixième des sept PR (007d-2 coupée en deux par l'humain le 2026-10-19).
Spec : `docs/spec/10-exploitation.md` §10.2. Conventions : `docs/design/09-conventions-du-socle.md` C1 à C3.
Règles : `.claude/rules/ci-deploy.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- CI : un seul workflow, `ci.yml` (C3), `permissions` globales en lecture ; `make check` construit les images en
  snapshot, sans push ; GoReleaser a `release` et `changelog` désactivés ; aucun tag dans le dépôt.
- `main` est fermé par un ruleset de branche (aucune règle sur les tags) ; chaque PR y entre en squash sous un
  titre conventionnel : le journal de `main` est une suite de commits conventionnels.
- Paquets GHCR : un paquet neuf naît privé ; sa visibilité n'hérite pas du dépôt ; le passage en public se fait
  dans ses réglages, par paquet, sans API.

**Terminé quand** (part de 007d-2a) : un merge sur `main` dont un commit change le binaire pose `vX.Y.Z`, publie la
release et les images `:vX.Y.Z` ; un merge `docs`, `chore`, `ci`, `test` ou `style` ne publie rien.

## Arbitrages

Décisions humaines du 2026-10-19 :

- **Déploiement par tag de release**, pas par SHA : « plus fiable et plus facile de faire un revert ».
  Amendés : C2, spec 10 §10.2, plan §007.
- **Une version par commit qui change le binaire** : `feat` → mineure ; `fix`, `perf`, `refactor`, `build`,
  `revert` → patch ; `!` ou `BREAKING CHANGE` → majeure, mineure tant que la version est `0.x` ; `docs`,
  `chore`, `ci`, `test`, `style` → rien. Un `revert` doit publier, sinon le retour arrière ne déploie jamais ; un
  `build(deps)` de Dependabot publie, ce qui est voulu.
- **Deux PR** : 007d-2a (tag et release), 007d-2b (déploiement par tag).

Fable a tranché le reste (2026-10-19) :

- **Un job, pas un second workflow** : un job `release` de `ci.yml` est « un workflow » au sens de la demande ;
  C3 tient. Un second fichier serait inerte : un tag créé par le `GITHUB_TOKEN` ne déclenche aucun workflow.
- **Tags `vX.Y.Z`** compatibles avec C2 : « ni tags préfixés » visait les tags par service (`migrate/v1.2.3`) ;
  un seul `vX.Y.Z` est « une version pour tout le dépôt ».
- **Pas de publication par SHA** : les images ne sont construites et poussées qu'à la release ; les PR et les
  pushes restent en snapshot. Un job, un login et une construction de moins.
- **Job `release`** : `needs: ci`, sur un push vers `main` seulement et si `ci` a réussi ; `permissions:
  {contents: write, packages: write}` à ce seul job ; `concurrency: {group: release}`, sans annulation du run en
  cours (deux merges rapprochés : le second voit le tag du premier ; trois : le run en attente est remplacé, le
  dernier bumpe pour tous). Corps : checkout complet, `next=$(go run ./tools/release)` (sortie vide : rien à
  publier, fin) ; `git tag "$next"` en local ; `docker login ghcr.io` par le `GITHUB_TOKEN` ; `make release`.
- **GoReleaser** : images taguées `{{ .Tag }}` (le même mot que le tag git) ; `release.target_commitish:
  "{{ .FullCommit }}"` : GitHub crée le tag distant à la création de la release, après le push des images ; un
  échec avant laisse zéro tag, et le run suivant recalcule la même version ; notes `changelog.use: github-native`.
  `gh release create` rejeté (un pas de plus, et le tag poussé d'abord).
- **Calcul de version** : `tools/release`, Go, une fonction pure `Next(current string, commits []string)
  (string, bool, error)` table-testée (l'erreur distingue un tag illisible de « rien à publier », qui
  sauterait sinon la release en silence) ; plomberie git (`describe --tags --abbrev=0`, `log`) dans la racine. `svu` rejeté :
  ses types de patch ne se configurent pas, et ce serait une exception C1 de plus pour trois expressions.
- **Premier tag** : `v0.1.0`, depuis un dépôt sans tag.
- **Visibilité GHCR** : geste unique par paquet, dans ses réglages ; deux paquets à la première release.
  Couplage inscrit dans CLAUDE.md (« Ajouter un `cmd/` »). Un jeton de lecture sur la VM est rejeté : un secret à
  faire tourner pour éviter deux clics.
- **Ruleset** : il ne vise que la branche ; le `GITHUB_TOKEN` peut créer le tag par la release. Un ruleset de
  tags futur devra exempter `github-actions`.
- **Preuve** : le merge de cette PR (`feat`) est sa preuve : `v0.1.0`, sa release, ses deux images. Une PR de suite
  la consigne.
- Noté : un `revert` d'un `feat` produit un patch (bizarrerie semver admise en `0.x`) ; sur `main`, `check`
  construit un snapshot que `release` reconstruit (pas de partage d'artefact, le plus court gagne).

## Design arrêté

- `internal/release` : `Next(current string, commits []string) (string, bool, error)` : `current` vide →
  `v0.1.0` s'il y a au moins un commit qui publie ; un `current` hors `vMAJOR.MINOR.PATCH` est une erreur ; chaque
  commit est un message complet (`%B`) ; type pris au titre (`type(scope)!: …`), rupture par `!` ou une ligne
  `BREAKING CHANGE:` en tête de ligne, et seulement pour un type qui publie (une rupture dans un `docs` ne
  livre rien) ; la plus forte montée l'emporte, quel que soit l'ordre ; en `0.x`, une rupture monte la mineure ;
  un titre non conventionnel ne publie pas. Cœur pur (ADR 0025), testé en table.
- `tools/release` : le dernier tag `v*` fusionné dans HEAD (`git tag --list 'v*' --merged HEAD
  --sort=-v:refname`, liste vide sans tag) et les messages depuis lui (`git log`, séparateur NUL) ; imprime la
  version suivante, ou rien ; sort en 1 sur un tag illisible.
- `.goreleaser.yaml` : `tags: ["{{ .Tag }}"]` ; `release.target_commitish: "{{ .FullCommit }}"` ;
  `changelog.use: github-native`.
- `Makefile` : `release` (`$(GORELEASER) release --clean`).
- `ci.yml` : job `release` (ci-dessus).
- Docs : C2, spec 10 §10.2, plan §007, CLAUDE.md (paquet public après la première release).
- Tests : table de `Next` (chaque type, `!`, `BREAKING CHANGE`, rupture en `0.x` et en `1.x`, montée la plus forte
  dans les deux ordres, « BREAKING CHANGE: » cité dans un titre, titre non conventionnel, aucun tag, rien à
  publier, tag illisible ou préfixé) vue tomber sous mutation ; `goreleaser check` ; `make check` construit encore
  le snapshot avec `{{ .Tag }}` ; `tools/release` sur le dépôt rend `v0.1.0`, sur une copie taguée rend la
  suivante, rien après un seul `docs`, et sort en 1 sur un tag `v1.2` ; `actionlint` sur `ci.yml`.
