# Step 007a — Livraison : images, `migrate`, squelette `deploy/k8s/`

Plan : `docs/plan/socle.md` §007, première des six PR. Règles : `.claude/rules/ci-deploy.md`,
`.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

Aucun `cmd/` n'existe. go-gateway construisait ses images par GoReleaser (`dockers_v2`, distroless
`static-debian12:nonroot`, un `Dockerfile` commun et un `Dockerfile.migrate` qui copiait les migrations),
les gardait par `internal/deploy` (une image par manifest et réciproquement, plancher de 12) et ne scannait
aucune image. Son `cmd/migrate` lisait `POSTGRES_URL` et prenait `up|down|version|force`.

**Terminé quand** (part de 007a) : une PR qui ajoute un `cmd/` sans son build, son image ou son manifest est
rouge ; une image qui porte une vulnérabilité connue est rouge ; `migrate` applique les migrations de
`control_plane` sous `migrator` et refuse de démarrer sans URL.

## Arbitrages

Découpage de la 007 (Fable, 2026-10-19), amendé et porté au plan : 007a, 007b-1 (`catalog-apply` : topics,
ACL), 007b-2 (`catalog-apply` : GRANT, reports de 006b), 007c (NetworkPolicy générées, alertes), 007d-1
(briques de l'environnement de test), 007d-2 (déploiement par SHA). La règle « chaque `cmd/` entre dans
GoReleaser et `deploy/k8s/` dans la même PR » met le squelette `deploy/k8s/` et le Job `migrate` ici.

Décisions humaines du 2026-10-19 :

- **NetworkPolicy générées** dans `deploy/k8s/`, sous `check-generated` (spec 10 §10.2, C3) ;
  `catalog-apply` ne les applique pas. Amendé : spec 02 §2.2, plan §007.
- **Environnement de test** : une VM k3s qui **tire** le SHA de `main` ; images publiques sur GHCR
  poussées avec le `GITHUB_TOKEN` ; aucun secret de déploiement dans GitHub ; retour arrière par
  `git revert`. Porté en 007d.

Les alertes dont la métrique source n'existe pas encore entrent avec le composant qui l'émet (Fable) :
séquencement porté au plan, pas une dette.

Fable a tranché les propositions le 2026-10-19 (A1, A4, A8 tenus ; A2, A3, A6, A7 amendés ; A5 en
partie rejeté) :

- **A1 — `migrate`** : `up` seul, sur `control_plane` seule, migrations embarquées
  (`migrations.ControlPlane()`). Ni `down`, ni `version`, ni `force` : up/down/up est prouvé en CI
  (006a), et un Job ne répare pas une base à la main. Journal JSON par le logger de 005b.
- **A2 — URL** : `POSTGRES_URL` entre dans `config.Infra` (forbidigo interdit à un `cmd/` de lire
  l'environnement) ; vide admis par `Load`, refusé par `migrate` : sans ce refus, pgx retombe sur `PG*`
  et `localhost`. Plafond : `message-state-svc` lira deux bases (spec 02 §2.3) et `migrate` en migrera
  quatre ; le champ se scinde avec la deuxième base.
- **A3 — Outils hors `tool`** : GoReleaser et grype par `go run <module>@vX.Y.Z`, version figée une fois,
  dans le `Makefile` ; la CI appelle `make`, aucune action à épingler. La directive `tool` tirerait dans
  `go.mod` les SDK des registres et des magasins qu'ils savent lire ou publier. Amendé : décision 9, C1.
- **A4 — Images** : `dockers_v2`, un seul `Dockerfile` (distroless `static-debian12:nonroot`, `USER
  65532:65532`, `ENTRYPOINT ["/app"]`) : les migrations sont embarquées, `Dockerfile.migrate` n'a plus de
  raison d'être. `linux/amd64` seul tant qu'aucune cible n'est arm. Registre
  `ghcr.io/martialanouman/sms-platform/<cmd>`. Sur PR : `release --snapshot`, sans push ; le push et le
  tag par SHA sont à 007d-2.
- **A5 — Scan** : grype sur chaque image du snapshot, bloquant dès une vulnérabilité de sévérité `low`,
  corrigée en amont ou non : « une vulnérabilité connue est rouge ». Une exception vivra dans
  `.grype.yaml` avec sa raison, et `debt: #N` si c'est un report ; aucune aujourd'hui, donc pas de
  fichier. Un scan qui ne trouve aucune image échoue.
- **A6 — Garde** : un paquet de tests `deploy/` lit `cmd/`, `.goreleaser.yaml` et `deploy/k8s/` : chaque
  `cmd/` a exactement un build, une entrée `dockers_v2` et un manifest du même nom, et réciproquement.
  Aucun plancher : l'égalité suffit. Pas de lecture du `Dockerfile` : `runAsNonRoot` fait refuser par
  Kubernetes une image root.
- **A7 — Manifests** : YAML brut, un fichier par service ou Job, image sans tag (posé en 007d-2). La
  garde vérifie chaque conteneur : `runAsNonRoot`, `readOnlyRootFilesystem`, image
  `ghcr.io/martialanouman/sms-platform/<nom du fichier>`, aucune variable hors de celles que lit
  `config`, et `POSTGRES_URL` par `secretKeyRef`. `config` expose la liste qu'il consomme : la garde ne
  recopie pas les noms. Pas de kubeconform : le schéma est prouvé par l'application réelle en 007d-2.
- **A8 — Hors 007a** : les gardes de go-gateway sur les `cmd/` supervisés (rappel de drain, composants
  lancés) arrivent avec le premier service supervisé : `migrate` n'en est pas un. Le TLS vers Postgres
  arrive avec la brique Postgres (007d-1). La dette #93 (OTLP) reste au premier composant : `migrate`
  n'émet aucune trace.

Limite assumée : Dependabot ne voit pas les versions du `Makefile` ; la base de grype, elle, est
téléchargée à chaque scan.

## Design arrêté

- `internal/platform/config` : une table nom → défaut (vide pour une variable sans défaut) est la liste
  fermée ; `Load` y prend ses défauts ; `Variables()` rend ses noms triés.
  `POSTGRES_URL` → `Infra.PostgresURL`, sans défaut.
- `cmd/migrate` : `run(ctx, config.Infra, *slog.Logger) error` refuse une URL vide, ouvre pgx, applique
  `goose` `Up` sur `migrations.ControlPlane()`, journalise chaque version appliquée ; `main` charge la
  configuration, sort en 1 sur erreur. Ni l'URL ni son mot de passe dans le journal.
- `Dockerfile`, `.goreleaser.yaml` (un build et une entrée `dockers_v2` par `cmd/`, `CGO_ENABLED=0`,
  `-trimpath`, ni archive, ni release, ni changelog).
- `Makefile` : `image` (`release --snapshot --clean`), `scan` (grype sur chaque image du snapshot,
  `--fail-on low`, échec si aucune image) ; les deux dans `check`.
- `deploy/k8s/migrate.yaml` : un Job, `restartPolicy: Never`, `backoffLimit: 0`, non-root, système de
  fichiers en lecture seule, `allowPrivilegeEscalation: false`, capacités retirées, `POSTGRES_URL` depuis
  le Secret `migrate`.
- `deploy/guard_test.go` : la garde A6–A7, appliquée à l'arbre et à des arbres fautifs de
  `deploy/testdata/`, un par règle.
- Tests d'intégration de `cmd/migrate` (pgtest) : `up` applique toutes les migrations sous `migrator`, et
  une seconde fois n'applique rien ; une URL vide est refusée avant toute connexion.
- `docs/design/09-conventions-du-socle.md` C1 : l'exception GoReleaser et grype.
