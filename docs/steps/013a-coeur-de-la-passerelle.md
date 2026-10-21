# Step 013a — Cœur et adaptateurs de la passerelle

Plan : `docs/plan/composants.md` §013, première des quatre PR de 013 (coupe : arbitrage 229). Arbitrages antérieurs : 1
à 228. Spec : `docs/spec/07-plan-de-controle.md` §7.3 à §7.5, §7.7 ; `docs/spec/09-contrats.md` ; ADR-0019 ;
`docs/design/05-autorisation-et-audit.md` §3.4, §4. Contrat : `contracts/openapi-admin.yaml` (`OperatorBearer`,
`list-audit-log`, `list-audit-changes`). Origine : `martialanouman/go-gateway`, `internal/auth`, `internal/adminapi`
(`audit.go`, `contract_test.go`). Règles : `.claude/rules/tests.md`, `go-code.md`. Mode : PR.

Ce document porte la coupe de **toute la 013** ; 013b à 013d y renvoient.

## Contexte

- Aucun `admin-api-svc` dans le dépôt. `control_plane` n'a que `audit_log` et `audit_changes` ; la déclaration de GRANT
  `contracts/grants/admin-api-svc.yaml` existe déjà (`audit_log` en `select, insert, delete`).
- L'émetteur du BFF (`internal/dashboard/token.go`, 012c) signe en ES256 : `alg`, `kid`, `typ` ; `iss` = URL publique
  du dashboard, `aud` = `gateway-admin`, `sub` = identifiant de l'opérateur, `scope` par espaces, `iat`, `exp`.
- `internal/platform/scopes` est généré des contrats ; la liste des dix scopes sensibles vit dans
  `internal/dashboard/roles.go`, que la passerelle ne peut pas importer (ADR-0025).
- L'origine écrit `audit_log` en deux temps (`Begin`, `Finish`) : la table est ici append-only, écrite une fois.

**Terminé quand**, contre un vrai Postgres : un jeton signé d'une clé du JWKS passe ; un `kid` inconnu rend 401 après
un rechargement ; chaque opération servie rend 403 au complément de son scope ; `list-audit-log` laisse sa ligne
`audit:read`, `status` 200, dans la transaction de sa lecture ; une écriture d'audit qui échoue rend 503 et jamais une
réponse de succès ; le contrat servi est un sous-ensemble du fichier.

## Arbitrages

Fable, 2026-10-21, sauf mention. La coupe (229) et 234 sont soumises à l'humain avec la PR.

229. **Coupe de 013 en quatre** :
     - **013a, cœur et adaptateurs** : vérificateur, scope par opération, middleware d'audit, `list-audit-log`,
       `list-audit-changes`, gardes de contrat ; prouvée en test.
     - **013b, service** : `cmd/admin-api-svc`, GoReleaser, `deploy/k8s`, Certificate `admin-api-svc-tls` (`server
       auth` et `client auth`), NetworkPolicy, alertes, `role-secrets` ; pas de PDB (spec 10 §10.2 ne le liste pas).
       Preuve sur `contabo169` : `/readyz`, un jeton personnel sous `audit:read` lit le journal et y laisse sa ligne.
     - **013c, proxy** : jeton de 5 min de l'opérateur, `/api/gw/*` limité aux gabarits du contrat Admin, mTLS client
       du BFF ; preuve `curl`.
     - **013d, écran Audit** : types TypeScript du contrat Admin, deux onglets ; preuve navigateur.
230. **huma v2** (spec 09) sur `net/http.ServeMux` par `humago`, sans routeur tiers. La garde compare, pour chaque
     opération servie : `operationId`, méthode, chemin, `security`, codes de réponse et noms des paramètres. Une
     opération servie absente du fichier échoue ; une opération du fichier non servie est tolérée jusqu'en 032. Le 503
     des middlewares n'est déclaré par aucune opération. Paie #52.
231. **Vérification sans bibliothèque JOSE** (miroir de 168) : `crypto/ecdsa` et `encoding/json`. Exigés : `alg` =
     `ES256`, `kid` connu, `iss` égal octet pour octet à la configuration, `aud` = `gateway-admin` (constante, comme
     l'émetteur), `exp` dépassé refusé sans tolérance, `sub` UUID. `scope` par espaces ; un scope hors `scopes.All`
     est ignoré.
232. **`kid` inconnu** : un rechargement, un seul en vol, au plus un par minute ; `kid` toujours inconnu ou JWKS
     injoignable → 401 (spec §7.7, qui prime sur le 503 de l'origine ; ADR-0019 amendé). Rien n'est lu au démarrage.
     Paie #53.
233. **Audit en une transaction, réponse tamponnée** : le middleware ouvre la transaction, pose `SET LOCAL app.actor`
     et `app.request_id`, la confie au handler et tamponne la réponse. Succès → `INSERT audit_log`, `COMMIT`, puis le
     tampon part. Échec du handler → `ROLLBACK`, puis la ligne du refus dans une transaction courte. Une écriture
     d'audit qui échoue → 503. `target` = chemin sans requête ; `outcome` NULL (il vient avec `get-message-content`) ;
     `request_id` généré par la passerelle (UUIDv7), jamais lu de la requête, rendu en `X-Request-Id`.
234. **Un 403 de scope est audité** (implémenteur, depuis spec §7.5 « à l'issue de la décision ») : le jeton est
     vérifié, son `sub` authentique ; un refus d'autorisation sur une opération auditée est un fait du plan de
     contrôle. Un 401 ne l'est pas : rien n'identifie l'appelant. Fable n'auditait aucun des deux.
235. **Audité** = méthode autre que `GET`, ou scope déclaré parmi les sensibles, lu du contrat. Les sensibles passent
     dans `internal/platform/scopes` (`Sensitive`), que `RequiresMFA` du dashboard lit aussi. Les lectures démasquées
     par `msisdn:reveal` : reportées (issue `debt`), déclencheur la première opération servie qui rend un `Msisdn`.
236. **Emplacement** (ADR-0025) : `internal/adminapi/` (principal, vérification pure sur un jeu de clés et une
     horloge, décision d'audit, ports) ; `internal/adminapi/huma/` ; `internal/adminapi/postgres/` (sqlc) ;
     `internal/adminapi/jwks/` (lecture du JWKS en `net/http`).
237. **`list-audit-changes`** : 404 si aucune ligne d'`audit_log` ne porte ce `request_id`, page vide sinon. Index
     `audit_log(request_id)` et `audit_changes(request_id, at, id)` : la clé `(at, id)` ne sert pas cette lecture.
238. **Dettes** : #52 et #53 payées ; #15 fermée comme caduque (aucun filtre par cible au contrat, table non
     partitionnée depuis 006a). Ouverte : audit des lectures démasquées (235).

## Design arrêté

- **Migration** `migrations/control_plane/00003_audit_by_request.sql` : les deux index de 237.
- **`internal/platform/scopes`** : `Sensitive` (manuel, à côté du généré) ; `internal/dashboard/roles.go` le lit.
- **Cœur `internal/adminapi`** : `Principal` ; `Verifier` (ports `Keys` et horloge), erreurs `ErrUnauthenticated` ;
  `Audited(method, scope)` ; port `AuditLog` (lecture paginée par curseur, lignes de changement, existence d'un
  `request_id`) et `Tx` (exécution sous acteur).
- **`internal/adminapi/jwks`** : `Keys` sur un client `net/http`, rechargement borné de 232.
- **`internal/adminapi/postgres`** : sqlc, transaction `SET LOCAL`, insertion d'audit, lectures paginées.
- **`internal/adminapi/huma`** : API huma ; middleware d'authentification et de scope lu de `Operation().Security` ;
  middleware d'audit de 233 ; handlers `list-audit-log`, `list-audit-changes`.
- **ADR-0019** : une ligne d'amendement (401 sur `kid` inconnu après rechargement).
- **Tests** : vérificateur (signature, `alg`, `kid`, `iss`, `aud`, `exp`, `sub`, scopes inconnus) ; rechargement
  borné ; garde de contrat ; complément de scope → 403 sur chaque opération servie ; Postgres réel sous le rôle
  `admin_api_svc` : ligne d'audit dans la transaction, échec d'audit → 503, refus du handler → rollback et ligne,
  403 audité, 401 non, filtres et curseur de `list-audit-log`, `list-audit-changes` 404 et page. Mutation manuelle,
  consignée.

## Écarts au design, constatés à l'implémentation

- **Transaction de la requête dans le contexte** : `adminapi.WithTx` et `TxFrom` (cœur), lus par les handlers ; toute
  opération servie, auditée ou non, s'exécute dans la transaction ouverte par le middleware. Un flux (026) devra en
  sortir, comme du tampon de réponse (marque `ponytail:` dans `huma/api.go`).
- **Curseur partagé** : `internal/platform/keyset`, sorti de `internal/dashboard/accesslog.go` ; deux consommateurs.
- **Tests bout en bout dans `internal/adminapi/postgres`** (`gateway_test.go`) : la sonde d'écriture du test passe par la
  transaction réelle, que seul ce paquet expose à ses tests. La signature de jetons de test vit dans
  `internal/adminapi/adminapitest`.
- **Modèle d'erreur réduit** à ce que les deux opérations atteignent : 422 `validation_error` avec `errors[]`, un code du
  catalogue porté par l'erreur, sinon 500. Une opération à corps (014a) y ajoutera 400 et 413.
- **Une opération sans exactement un scope** rend 401 à tout jeton : l'échec reste fermé si la garde de contrat manquait.
- **L'adaptateur Postgres** a été écrit dans la même commande que son test, sans rouge lu ; compensé par les mutations du
  SQL généré.

## Mutations

À la main, par un script qui copie, mute, teste et restaure (`cmp`), et signale un mutant qui ne compile pas. Vérificateur
13/13 ; JWKS 13/13 ; curseur 3/3 ; cœur de l'audit 8/8 ; SQL généré 11/11 ; adaptateur Postgres 8/8 ; couche huma 18/19,
modèle d'erreur et handlers 8/8 après retraits, cible et tags JSON 7/7. Équivalents gardés : `>= 400` contre `> 400` (aucun 400 ne sort), l'échec
de décodage base64 du curseur (le dernier quantum perdu fait tomber l'UUID). Retirés parce qu'équivalents : les filtres
`kty`/`crv` et de longueur du JWKS, le plafond explicite du corps, `!found` du curseur, la branche 400, le masque du
message 500 (huma n'y met jamais la cause), `ContentType`. Survivants qui ont révélé un cas réel, tués : une signature
complétée de zéros, un jeton à quatre segments, un jeton brut sans `Bearer`, le `$schema` d'huma.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant.

1. **JWKS vidé par un corps d'erreur** (haute, sécurité ; mon retrait du contrôle de statut) : un `502` ou un `200` sans
   clé remplaçait le cache par un jeu vide, 401 pour tous une minute. Un jeu sans clé est désormais un échec ; le statut
   n'ajoute rien de plus (mutant équivalent, retiré).
2. **Rechargement sous le contexte de l'appelant** (moyenne) : un appelant parti faisait échouer le rechargement et
   bloquait la minute. Il tourne sous `WithoutCancel`, borné par son propre délai.
3. **Tests creux** : la cible d'un appel avec requête, et chaque champ des pages, sont désormais affirmés ; les mutations
   (`RequestURI`, tags JSON) tombent.
4. **Code en trop retiré** : les helpers de `verify_test` recopiés d'`adminapitest`, la constante `operator` redéclarée,
   `Config.Log`, la branche d'erreur de `uuid.NewV7`, un commentaire.
5. **Gardés, avec raison** :
   - la ligne d'une lecture auditée dans la transaction de sa lecture n'a pas de test qui la distingue d'une ligne
     écrite juste après le `COMMIT` : seul le refus d'écrire (503) est prouvé ;
   - la composition `jwks.Keys` + vérificateur + huma, et « rien n'est lu au démarrage », se prouvent au câblage de 013b ;
   - un `Issuer` vide et un `Log` nil du JWKS sont des erreurs de configuration, refusées par la validation de 013b ;
   - la regexp d'UUID reste écrite deux fois (vérification d'un `sub`, d'un curseur) : la partager lierait le cœur de
     l'autorisation au paquet de pagination ;
   - un 503 après un `COMMIT` dont l'accusé se perd peut cacher une mutation faite : inhérent.
