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

## Écarts au design, constatés à l'implémentation

- **Pas de cas d'usage `Me`** : `Resolve` rend `Current`, que 011a-3 traduit en DTO (permissions vides, non élevée,
  `IdleTimeout` en secondes) ; un cas d'usage qui ne ferait que recopier serait du code en trop.
- **L'exclusion du code généré vise gosec (G101)**, pas revive : sqlc nomme `SetPassword` la requête qui pose
  `password_hash`, et revive ne signale rien.
- **sqlc** : `2 ^ attempts` casse sa réécriture des paramètres nommés ; le backoff s'écrit `power(2, attempts)` sous
  `sqlc.arg`. Les uuid, textes nullables et `timestamptz` sont surchargés en `string`, `*string` et `time.Time`.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité, tests creux, code en trop). Un bloquant, tranché par l'humain ; 30 constats.

1. **TLS exigé toujours** (humain, 2026-10-20 ; révise la décision B et l'arbitrage 44) : un relais qui n'offre pas
   STARTTLS est refusé. Sinon, un tiers sur le chemin retirait STARTTLS de l'EHLO, et le lien de reset partait en
   clair. Mailpit reçoit en 011a-3 un certificat de la CA interne.
2. **Lien d'accès en une transaction** (révise l'arbitrage 40) : `ConsumeLink`, puis la politique, puis le hachage,
   puis le mot de passe ; un refus de politique ou un hacheur occupé annulent la transaction, et le lien reste
   valide. `LinkUsable` disparaît, avec ses filtres qui recouvraient ceux de `ConsumeLink`. Deux usages concurrents se
   sérialisent sur la ligne. Le coût : une connexion tenue le temps d'un argon2, dix au plus.
3. **Un 429 sur l'e-mail garde l'incrément de la source** : le refus sort après le commit.
4. **Une adresse source invalide est refusée** par `Login` : une adresse zéro donnerait un sujet partagé par tous.
5. **Curseur** : l'identifiant doit être un uuid (36 caractères hexadécimaux et tirets aux bonnes places), sinon
   `ErrBadCursor`, au lieu d'un échec de conversion dans Postgres.
6. **Les sessions se ferment à tout usage d'un lien**, activation comprise : un compte sans mot de passe n'en a pas,
   la branche sur le genre de lien était inutile.
7. **Tests resserrés** : un second opérateur témoin, avec son lien et ses sessions, que chaque écriture ciblée laisse
   intact (`SetPassword`, `CloseSessions`, `Touch`, `MarkFailed`, `MarkSent`, `CloseSession`) ; le plus ancien lien
   dû part d'abord ; deux livraisons concurrentes envoient deux liens distincts (`SKIP LOCKED`) ; l'objet de
   l'activation ; les curseurs mal formés après le base64 ; tous les champs de `Current` ; un e-mail stocké en
   majuscules se retrouve.
8. **Retraits** : `SessionView` embarque `Current` ; le `ctx` ignoré du hacheur ; le `max` inatteignable de
   `Reserve` ; le paramètre `displayName` (012 l'ajoutera avec ce qui l'édite) ; les cas redondants du PHC ;
   `slots_internal_test.go`, rattaché à `password_test.go` ; `RETURNING failures`.
9. **Gardés tels quels, transmis à 011a-3 ou 012** :
   - La source compte aussi les succès (arbitrage 41) : derrière un NAT, cinq connexions en 15 min verrouillent
     la sixième.
   - Un tiers peut verrouiller l'e-mail d'un opérateur, et un reset n'oublie pas ce compteur (15 min au plus).
   - Un hachage illisible rend 500 : une donnée corrompue doit se voir ; #17 changera les paramètres par
     re-hachage.
   - `session_refused` s'écrit à chaque requête jusqu'à la purge.
   - Un `logout` sur une session morte et non purgée est journalisé.
   - En 012 : désactiver un compte devra supprimer ses sessions et son lien, faute de quoi une réactivation rapide
     les ferait revivre.
   - En 011a-3 : `Current` ne dit rien de l'élévation, donc une session de premier facteur n'atteint que `/auth/*` ;
     un lien en échec après dix tentatives doit devenir visible, par une métrique.
   - `IdleTimeout` et `lockWindow` valent tous deux 15 min : leur permutation est indétectable.
