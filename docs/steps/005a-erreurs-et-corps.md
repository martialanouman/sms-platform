# Step 005a — Bibliothèques : erreurs, corps

Plan : `docs/plan/socle.md` §005, première des cinq PR. Spec : `docs/spec/09-contrats.md` §9.3, I1
(`docs/spec/README.md`), `docs/spec/02-architecture.md` §2.4. Mode : PR.

## Contexte

Portées de `go-gateway` (`internal/platform/errors`, `internal/platform/msg`) et adaptées à la spec. Le
catalogue de go-gateway suit l'ancien guide (`external_billing_unavailable`, statut SMPP propriétaire de
`insufficient_credit`, `Retryable`) ; la table §9.3 fait foi désormais.

**Terminé quand** (part de 005a) : le catalogue et le corps ont leurs tests ; la correspondance avec §9.3
et la garde de `Reveal` ont été prouvées par une mutation.

## Arbitrages

La spec tranche le contenu ; ce qui reste est de la forme :

- **Catalogue** écrit à la main dans `internal/platform/errors` (territoire de `.claude/rules/errors.md`) :
  une sentinelle `Err…` par code de §9.3, de type `Code` (qui implémente `error`), et son statut HTTP et
  SMPP, zéro s'il n'en a pas. Un test le compare ligne à ligne à la table §9.3 (code, HTTP, SMPP). Les
  `enum` des contrats sont dans §9.3 (`make openapi`), donc dans le catalogue, par transitivité.
- **Une seule lecture de §9.3** : le parseur de `internal/openapi` rend les lignes (code, HTTP, SMPP) au
  lieu des seuls codes ; une ligne à plusieurs codes porte une valeur commune ou une valeur par code, dans
  l'ordre ; « — » se lit valeur par valeur ; tout autre compte est une erreur.
- **Frontière** : `HTTP(err)` rend le code et le statut d'une réponse REST, `SMPP(err)` le
  `command_status`. Une erreur sans code, ou dont le code n'a pas de surface sur ce protocole, sort en
  `internal_error` : 500, `ESME_RSYSERR`. Les codes d'après l'ACK n'ont pas de surface synchrone (§9.3).
- **Abandonné de go-gateway** : `Retryable` (la spec n'en dit rien), le statut propriétaire de
  `insufficient_credit` (« — » au §9.3), le mapping inverse SMPP → code (au connecteur), les adaptateurs
  huma et gRPC (avec leurs composants), `Len`, `IsEmpty`, `NewBodyString`.
- **Corps** : `msg.Body` (`internal/platform/msg`) ne garde qu'une fonction non exportée qui rend le
  clair : `fmt` (et `slog` texte, qui passe par lui) rend une fonction par son adresse, à toute
  profondeur et pour tout verbe, champ non exporté d'une struct compris. Un pointeur ne suffit pas : un
  verbe inadapté (`%s`) le fait imprimer en `%v`, contenu compris. `encoding/json` et `slog` JSON ne
  voient aucun champ exporté. Aucune méthode de rendu. La valeur zéro est un corps vide.
- **Garde de `Reveal`** : le clair sort par une fonction, `msg.Reveal(b)`, que `forbidigo` (golangci-lint,
  `analyze-types`) refuse partout. Une fonction ne se promeut pas par embedding et ne passe pas par une
  interface, contrairement à une méthode. Chaque appel admis est une exclusion de `.golangci.yml` qui
  nomme son fichier et sa raison : l'audit. Seule exclusion aujourd'hui, le test du paquet.
- **Revue, deuxième tour** : l'audit ne vaut que si aucune exclusion ne lui échappe. `make lint` refuse
  tout `nolint` et tout `go:linkname` dans le Go du dépôt (il n'y en a aucun) ; les fichiers générés
  sont lintés (`generated: disable`), le `gosec` des `.pb.go` exclu par chemin. Le paquet `msg` lit son
  propre champ sans garde : il ne contient que `Body` et `Reveal`.

## Design arrêté

- `internal/openapi` : `SpecErrors(md []byte) ([]SpecError, error)`, `SpecError{Code, HTTP, SMPP string}`,
  remplace `SpecCodes` ; `tools/openapi` en tire les codes.
- `internal/platform/errors` : `Code`, sentinelles `Err…` de tous les codes de §9.3, statuts SMPP
  nommés, `HTTP(error) (Code, int)`, `SMPP(error) uint32`.
- `internal/platform/msg` : `Body`, `NewBody([]byte) Body`, `Reveal(Body) []byte`.
- `.golangci.yml` : `forbidigo` activé, `analyze-types`, motif `msg.Reveal` du paquet, exclusion du
  test du paquet ; fichiers générés lintés. `Makefile` : `lint` refuse `nolint` et `go:linkname`.
- Vérification du critère : catalogue et §9.3 mutés tour à tour (code, HTTP, SMPP) rouges ; un appel à
  `Reveal` hors exclusion, direct, par valeur de fonction, dans un fichier généré ou sous `nolint`, rend
  `make lint` rouge ; aucune sortie `fmt`, `slog` (texte, JSON) ni `encoding/json` ne contient le clair,
  `Body` nu, en champ exporté ou non exporté.
