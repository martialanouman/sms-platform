# Step 011a-2b — Dashboard : cœur et adaptateurs

Plan : `docs/plan/composants.md` §011, seconde des deux PR de 011a-2. Arbitrages : `docs/steps/011a-1-prometheus-et-alertmanager.md`
(1 à 22) et `docs/steps/011a-2a-schema-par-base.md` (23 à 36). Spec : `docs/spec/08-dashboard.md` §8.3, §8.9. Contrat :
`contracts/openapi-bff.yaml` (`login`, `setPasswordFromAccessLink`, `me`, `reportActivity`, `logout`, `listAccessLog`).
ADR : 0025. Origine : `go-gateway-bo` (`internal/auth`, `internal/session`, `internal/store`, `cmd/dashboard/mail.go`).
Règles : `.claude/rules/go-code.md`, `.claude/rules/migrations.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 011a-2a a livré la base `dashboard` (cinq tables, porte du journal) et le rôle `dashboard` ; aucun code ne la lit.
- Le dépôt n'a encore aucune requête sqlc ; `golang.org/x/crypto` n'y est qu'indirect.
- Dettes du territoire : #8 (sessions expirées jamais purgées), #11 (verrou du premier opérateur), #18 (fenêtre
  d'oubli écrite deux fois), #19 (compteurs jamais purgés), #22 (garde inatteignable) : soldées ici. #12 (fermeture du
  pool) et #20 (binaire sans Node) : en 011a-3, avec le `cmd/`. #17 (calibration d'argon2id) : reportée, inchangée.

**Terminé quand** : contre un vrai Postgres, sous le rôle `dashboard`, un opérateur né par le bootstrap reçoit son
lien par SMTP, s'en sert pour poser un mot de passe, se connecte, voit sa session close passé 15 min sans geste ou 12 h
après la connexion ; chaque fait de 011a a sa ligne au journal.

## Arbitrages propres à 011a-2b

Tranchés par l'agent, sans contredire la spec ni les arbitrages 1 à 36.

37. **Tests des cas d'usage contre le vrai Postgres** (`pgtest`, rôle `dashboard`) plutôt que contre un double du
    port : la mutation d'une constante (15 min, 12 h, 5 échecs) tombe sur une ligne antidatée par SQL, comme celle du
    prédicat (arbitrage 26). Seul le `Mailer` a un double, port vers un tiers.
38. **Créneaux de hachage non bloquants** : les dix créneaux pris, l'appel rend `ErrBusy` aussitôt (le contrat :
    « every argon2id hashing slot is busy »), au lieu d'attendre l'échéance de la requête comme l'origine.
39. **Un port de transaction** : `Store.Tx(ctx, func(Tx) error)` ; `Tx` porte les opérations fines, le cœur les
    compose (le `reset` ferme les sessions dans la transaction de l'usage du lien, arbitrage 2). L'adaptateur les
    implémente sur les requêtes sqlc, sous un `pgx.Tx`.
40. **Lien d'accès en deux temps** : sa validité se lit hors transaction avant la politique et le hachage (le contrat :
    jeton jugé d'abord, lien intact sur 422 ou 503), puis se consomme dans la transaction qui pose le mot de passe ;
    une course entre deux usages rend le même 410.
41. **Compteurs** : chaque tentative est réservée sur la source puis sur l'e-mail avant tout hachage ; un succès oublie
    le compteur de l'e-mail, jamais celui de la source (un compte valide ne blanchit pas une adresse). Verrouillé : 429,
    `Retry-After` arrondi à l'entier supérieur, au moins 1.
42. **Journal** : `login` (succès ; échec avec `actor` nul sur e-mail inconnu, l'identifiant sinon), `logout` (session
    existante seulement), `access_link_requested` (à la mise en file, `actor` nul au bootstrap, `target` l'opérateur),
    `access_link_used`, `session_refused` (sceau valide, session close, expirée ou d'un compte désactivé). Rien sur un
    429, un sceau faux ou un cookie absent.
43. **Livraison** : `DeliverNext` prend sous `FOR UPDATE SKIP LOCKED` la ligne due la plus ancienne (sans jeton,
    moins de 10 tentatives), tire le jeton, envoie, puis pose digest, échéance et `sent_at` ; un échec d'envoi compte la
    tentative et repousse à `least(30 s × 2^tentatives, 10 min)`. L'échéance part de l'envoi.
44. **SMTP** : `net/smtp` ; échéance de 10 s sur la connexion ; STARTTLS si offert, vérifié par la CA reçue ; `PLAIN`
    si un utilisateur est donné, refusé sans TLS par l'adaptateur ; erreur réduite au code SMTP, sans adresse ni
    corps. Le corps du message porte le lien, `DASHBOARD_PUBLIC_URL + "/access#" + jeton`.

## Design arrêté

- **`internal/dashboard`** (cœur, aucune I/O) :
  - `password.go` : `CheckPassword` (règles manquantes, en clair pour le 422), `Hasher` (dix créneaux, PHC
    `m=65536,t=3,p=4`, sel de 16 octets, clé de 32, décodage borné, `VerifyDummy` sous le même créneau), `ErrBusy`.
  - `keys.go` : `NewKeys(secret)` refuse moins de 32 octets ; `Seal`/`Open` du cookie sous `session:`,
    `Subject(domaine, valeur)` sous `source:` et `email:`.
  - `token.go` : jeton de 256 bits en base64url, digest SHA-256.
  - `source.go` : adresse source normalisée (IPv4 telle quelle, IPv6 au /64).
  - `store.go` : ports `Store`, `Tx`, `Mailer` et types de domaine (`Operator`, `Session`, `Entry`, `LinkKind`).
  - Cas d'usage (arbitrage 35) : `bootstrap.go`, `login.go`, `accesslink.go`, `session.go` (`Resolve`, `Activity`,
    `Logout`, `Me`), `accesslog.go`, `delivery.go`, `purge.go`. Erreurs de domaine que 011a-3 mappera : `ErrUnauthenticated`,
    `RateLimited{RetryAfter}`, `ErrBusy`, `ErrLinkInvalid`, `PolicyError{Missing}`, `ErrBadCursor`.
- **`internal/dashboard/postgres`** : `sqlc.yaml` à la racine, requêtes sous `queries/`, généré sous `sqlc/` ; `New(pool)`
  implémente `Store`. Exclusion `revive` nommée sur le généré dans `.golangci.yml`.
- **`internal/dashboard/smtp`** : `Mailer{Addr, From, Username, Password, CA}`.
- **`.golangci.yml`** : `golang.org/x/crypto/argon2` dans la liste blanche du cœur.
- **`contracts/grants/dashboard.yaml`** resserré sur ce que les requêtes utilisent ; fichiers régénérés.
- **Tests** : unitaires du cœur pur (politique, PHC, créneaux, sceau, sujets, source) ; cas d'usage contre Postgres
  sous le rôle `dashboard` (bootstrap à deux répliques, connexion et ses refus à temps égal par le même chemin,
  verrouillage et `Retry-After`, lien valide, invalide, expiré, remplacé, compte désactivé, politique, `reset` qui
  ferme les sessions, inactivité et 12 h par lignes antidatées, `Resolve` qui ne prolonge pas, `Activity` qui
  prolonge, journal et curseur, livraison et backoff, purge) ; SMTP contre `axllent/mailpit:v1.31.4` (clair, STARTTLS,
  `PLAIN` refusé sans TLS, `PLAIN` accepté sous TLS) ; aucun corps de mot de passe ni jeton dans les erreurs.
