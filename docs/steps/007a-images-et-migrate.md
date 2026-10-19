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
- **A4 — Images** : `dockers_v2`, un seul `Dockerfile` (distroless `static-debian12:nonroot`, qui porte
  déjà l'utilisateur 65532, `ENTRYPOINT ["/app"]`) : les migrations sont embarquées, `Dockerfile.migrate` n'a plus de
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
  `config`, ni `envFrom` qui les cacherait, et `POSTGRES_URL` par `secretKeyRef`. `config` expose la liste qu'il consomme : la garde ne
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
  `goose` `Up` sur `migrations.ControlPlane()` sous le verrou de session Postgres de goose (deux Jobs
  simultanés se succèdent : sans lui, goose n'en pose aucun), journalise chaque version appliquée ; `main`
  charge la configuration, sort en 1 sur erreur. Ni l'URL ni son mot de passe dans le journal.
- `internal/platform/pgtest` : `Empty` rend la base `control_plane` et `migrator` sans migration ;
  `ControlPlane` la migre.
- `Dockerfile`, `.goreleaser.yaml` (un build et une entrée `dockers_v2` par `cmd/`, `CGO_ENABLED=0`,
  `-trimpath`, ni archive, ni release, ni changelog).
- `Makefile` : `image` (`release --snapshot --clean`), `scan` qui en dépend (grype sur chaque image du
  snapshot qu'il vient de produire, `--fail-on low`, échec si aucune image) ; `scan` dans `check`.
- `deploy/k8s/migrate.yaml` : un Job, `restartPolicy: Never`, `backoffLimit: 0`, non-root, système de
  fichiers en lecture seule, `allowPrivilegeEscalation: false`, capacités retirées, `seccompProfile`
  `RuntimeDefault` (admis par Pod Security « restricted »), `POSTGRES_URL` depuis le Secret `migrate`.
- `deploy/guard_test.go` : la garde A6–A7, appliquée à l'arbre, puis à une copie de l'arbre faussée
  par règle (un remplacement dans un fichier réel, ou un fichier retiré). En plus des noms : le build
  d'un `cmd/` compile `./cmd/<nom>` avec `CGO_ENABLED=0` (un binaire lié dynamiquement ne démarre pas
  dans l'image statique), son image ne bâtit que ce build et ne se publie que sous le registre. Côté
  manifests : tout fichier de `deploy/k8s/` est un `.yaml` (un `.yml` serait appliqué sans être gardé) ;
  chaque document est un `Job` avec au moins un conteneur, les autres sortes (`Deployment` compris)
  entrent avec leur premier manifest et leur règle ; chaque conteneur, init compris, porte lui-même `runAsNonRoot`,
  `readOnlyRootFilesystem` et `seccompProfile` `RuntimeDefault`, au niveau du conteneur, plus strict
  que le minimum de Kubernetes.
- Tests d'intégration de `cmd/migrate` (pgtest) : `up` applique toutes les migrations sous `migrator`, et
  une seconde fois n'applique rien ; deux `run` simultanés réussissent tous les deux, sur une base dont
  la table de version existe déjà (cas de tout déploiement après le premier ; sur une base vide, la
  reprise de goose sur la création de cette table sérialise les deux `run` et le test ne prouve rien) ; le journal ne
  porte ni l'URL ni le mot de passe ; une URL vide est refusée avant toute connexion.
- `docs/design/09-conventions-du-socle.md` C1 : l'exception GoReleaser et grype.

## Revue

**Tour 1** (trois axes, 18 constats) :

- Retenus : garde sans conteneur ni sorte fermée (bloquant), `.yml` non gardé, cas `initContainers`,
  `CGO_ENABLED=0` non gardé, mot de passe non cherché dans le journal, `seccompProfile` absent, aucun
  verrou entre deux `migrate`, `scan` sur un snapshot périmé, boucle de copie de l'arbre dupliquée, `USER`
  redit, `GOTOOLCHAIN=auto` sans raison (gardé avec sa raison : setup-go pose `GOTOOLCHAIN=local` en CI, où Go
  1.26 refuse GoReleaser, qui exige 1.27 ; le retirer a cassé la CI de la PR), contrôles de la garde et
  `pgtest.Empty` absents du design, phrase « exactement N images » du plan.
- Écartés : un test qui relit les options du `Makefile` (`--fail-on low`) comparerait à la constante
  testée ; le comportement est prouvé par exécution (base `alpine:3.10` rouge, snapshot vide rouge).
  `-trimpath` n'est pas gardé : aucune propriété d'exécution n'en dépend. Une borne de temps du Job et
  le tag de l'image relèvent de la séquence de déploiement (007d-2, porté au plan). `migrate` ne couvre
  que `control_plane` : les autres bases arrivent avec leur composant (plan §006). Une fonction de
  migration partagée entre `pgtest` et `migrate` éviterait quatre lignes pour un export de plus.

**Tour 2** (deux axes, 4 constats mineurs, aucun bloquant) :

- Retenus : `Deployment` admis sans cas qui l'exerce (retiré de la liste fermée), `MkdirAll` redit dans
  le test de la garde, préparation du test de course absente du design.
- Écarté : un `---` final refusé (`kind ""`) : le refus est sûr et visible ; le message cite la sorte
  entre guillemets.
