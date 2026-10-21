# Step 013c — Proxy `/api/gw/*` et jeton de l'opérateur

Plan : `docs/plan/composants.md` §013, troisième des quatre PR de 013 (coupe : arbitrage 229). Arbitrages antérieurs : 1
à 250 (`docs/steps/013a-coeur-de-la-passerelle.md`, `docs/steps/013b-service-admin-api.md`). Spec :
`docs/spec/08-dashboard.md` §8.2 à §8.4 ; `docs/spec/07-plan-de-controle.md` §7.4, §7.7. Décisions : design 05 §3.3,
§3.4 ; ADR-0019. Contrats : `contracts/openapi-bff.yaml` (préambule « Proxy », « Cross-site requests »),
`contracts/openapi-admin.yaml`. Règles : `.claude/rules/go-code.md`, `tests.md`, `ci-deploy.md`, `contracts.md`. Mode : PR.

## Contexte

- Le BFF émet le jeton personnel (012c) ; `IssueOperatorToken` a été renvoyé ici (arbitrage 169).
- `admin-api-svc` est déployé (013b) et n'admet que le SAN `dashboard` ; `dashboard-tls` porte `client auth`.
- Le routeur du BFF valide chaque requête `/api/` contre le contrat BFF, qui n'a aucun chemin `/gw`.

**Terminé quand**, `curl` sur `contabo169` avec le cookie d'une session élevée : `GET /api/gw/admin/audit-log` rend la
page de la passerelle, et le second appel montre la ligne du premier sous le `sub` de l'opérateur ;
`/api/gw/admin/nope` et `/api/gw/admin/stream/metrics` → 404 `not_found` du BFF, sans ligne au journal de la passerelle ;
un rôle sans `audit:read` → 403 `forbidden_scope` de la passerelle et sa ligne ; une session non élevée → 403
`mfa_required` ; un `POST` sans `Origin` → 403 `forbidden_origin` ; la passerelle arrêtée → 503 `service_unavailable`.

## Arbitrages

Fable, 2026-10-21, sauf mention. 252 et 254 tranchés par l'humain le même jour.

251. **Pas de coupe** : un handler, un client TLS, une variable, un cas `netpol`.
252. **`IssueOperatorToken(ctx, c Current)`** (cœur, `token.go`) : claims de 169, `exp` à 5 min ; `scope` =
     `c.Permissions ∩ scopes.All`, trié ; aucune exclusion (celles du jeton personnel lui sont propres, 170) ; une
     intersection vide émet quand même, la passerelle décide (I12). Aucune ligne au journal d'accès. **Signé à chaque
     requête relayée, sans cache** (humain) : un changement de rôles vaut dès l'appel suivant, aucun état entre
     réplicas. ADR-0019 amendé d'une ligne.
253. **Place** : `/api/gw/` monté à côté de `/api/`, hors de la validation du contrat BFF. Le proxy enchaîne : `no-store`,
     origine sur méthode non sûre, session (401), garde `mfa_required` (`RequiresMFA`) ; aucun contrôle de permission
     (I12).
254. **Média** (humain) : sur méthode non sûre, le proxy exige l'un des médias que déclare l'opération Admin appariée,
     sinon 415. La phrase « `application/json` … the proxy included » du contrat BFF est corrigée (prose seule) :
     `import-exact-routes` prend `text/csv`.
255. **Chemins** : le contrat Admin chargé une fois par kin-openapi ; chemin ou méthode absents → 404 `not_found`, aucun
     jeton signé. Les trois flux `x-websocket-message` → 404 jusqu'en 026.
256. **Relais** : `httputil.ReverseProxy` ; préfixe `/api/gw` retiré ; `Cookie` retiré, `Authorization: Bearer` posé
     (celui du navigateur écrasé) ; aucun `X-Forwarded-*` ; réponse relayée telle quelle (`X-Request-Id` compris).
     Transport `infra.TLS.Client()`, SAN vérifié depuis l'hôte, `ResponseHeaderTimeout` 30 s. Erreur de transport → 503
     `service_unavailable`, journalisée sans corps (I1, un test le prouve). Pas de plafond de corps (contrat BFF).
257. **Configuration** : `ADMIN_API_URL=https://admin-api-svc:8443`, `https://` exigé au démarrage ; `netpol` en dérive
     la sortie du dashboard vers les pods `admin-api-svc` sur 8443.
258. **Emplacement** (ADR-0025) : jeton au cœur ; proxy dans `internal/dashboard/http/proxy.go`, câblé par
     `cmd/dashboard`.

## Design arrêté

- **`contracts/openapi-bff.yaml`** : la phrase du média, corrigée (254).
- **`internal/platform/config`** : `ADMIN_API_URL`.
- **`internal/dashboard`** : `IssueOperatorToken`.
- **`internal/dashboard/http/proxy.go`** : le proxy (253 à 256) ; `Config` reçoit le client et l'URL de la passerelle.
- **`cmd/dashboard`** : validation de `ADMIN_API_URL`, transport mTLS, câblage.
- **`internal/netpol`**, **`deploy/k8s/dashboard.yaml`**, `networkpolicies.yaml` régénéré.
- **ADR-0019** : une ligne d'amendement (252).
- **Tests** : jeton (claims, scopes, aucune exclusion) ; proxy contre une passerelle de test sous mTLS : relais et
  réécriture, en-têtes posés et retirés, 401, `mfa_required`, `forbidden_origin`, 415 et média accepté de l'opération,
  404 hors contrat et pour les flux sans appel amont, 503, aucun octet du corps au journal, réponse amont relayée telle
  quelle (403 de la passerelle compris) ; `cmd/` (refus de `ADMIN_API_URL`) ; `netpol`.

## Écarts au design, constatés à l'implémentation

- **Session partagée** : la lecture du cookie et `Resolve` sont sorties de la garde des opérations du BFF
  (`server.current`) ; le proxy et cette garde l'appellent tous deux. `unsafe` est partagé de même.
- **Opération sans corps** : le proxy exige `application/json` sur une méthode non sûre dont l'opération Admin ne
  déclare aucun corps, comme le BFF pour ses propres opérations.
- **Transport construit par `dashboardhttp.GatewayTransport`** (délais de connexion, de poignée TLS et d'en-têtes) : les
  tests du proxy relaient par lui, sous mTLS, vers une passerelle de test au certificat client exigé. Le `cmd/` ne
  fait que l'appeler ; son test de bout en bout n'a pas d'OpenBao, le relais y rend 503 avant le transport, et
  `Gateway` n'y est donc pas exercé : la preuve sur `contabo169` le couvre.

## Mutations

À la main, par le script de 013a. Jeton de l'opérateur 4/4 ; proxy 13/13 ; configuration 1/1 ; validation de
`ADMIN_API_URL` 1/1 ; `netpol` 3/3.

## Preuve sur `contabo169`

Jouée le 2026-10-21 depuis la branche, sur le cluster en v0.27.0 (`admin-api-svc` déployé par la release, paquet GHCR
public) : image `dashboard` construite localement, importée sous `proof-013c` ; timer suspendu, puis réarmé.

1. NetworkPolicy et manifest appliqués ; `rollout status` réussi ; le dashboard redémarré porte `dashboard-tls` avec
   `client auth`.
2. Sans session : `GET /api/gw/admin/audit-log` 401 `unauthenticated` ; `POST` sans `Origin` 403 `forbidden_origin` ;
   `/api/gw/admin/nope` et `/api/gw/admin/stream/metrics` 404 `not_found` ; `Cache-Control: no-store`.
3. Session élevée de l'opérateur, interrogée toutes les 10 s depuis la console pendant que l'état change :
   - `Propriétaire` : 200, la ligne d'audit porte le `sub` de l'opérateur et un `X-Request-Id` de la passerelle ;
   - rôle `Reporting` seul : 403 `forbidden_scope`, avec un `X-Request-Id` : refus de la passerelle, audité ;
   - session désélevée : 403 `mfa_required` sans `X-Request-Id` ; la ligne la plus récente du journal reste le dernier
     403 de la passerelle : rien n'a été relayé ;
   - `admin-api-svc` à zéro réplique : 503 `service_unavailable` ; le journal du dashboard ne porte que l'opération et
     l'erreur de connexion ;
   - réplique rendue : 200.

Rôles et élévation de l'opérateur rétablis ; `dashboard` reste sous `proof-013c` jusqu'à la release.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant.

1. **Transport sans délai de connexion ni de poignée** (moyenne) : une passerelle muette retenait chaque requête sans
   rendre le 503 promis ; `GatewayTransport` borne chaque étape, un test le prouve contre un écouteur muet.
2. **Segments `.` et `..` relayés** (basse) : `/admin/customers/%2e%2e` atteignait la passerelle ; le proxy refuse tout
   chemin que `path.Clean` change.
3. **Client parti journalisé comme passerelle injoignable** (basse) : rien n'est journalisé quand le contexte de la
   requête est déjà fini.
4. **Tests creux** : l'ordre « contrat, origine, média avant la session » est prouvé sans cookie ; le signataire en
   panne est joué passerelle vivante, sans appel amont ; un `Authorization` forgé est écrasé ; aucun jeton n'est signé
   pour un refus ; le 200 relayé est comparé octet pour octet, `Content-Type` compris ; la passerelle de test exige
   le mTLS.
5. **Code en trop retiré** : la signature des claims partagée entre jeton personnel et jeton de l'opérateur, la branche
   `RequestBody.Value == nil`, la constante `relayPrefix`.
6. **Dette #167** : le `ReadTimeout` de 10 s borne un import relayé (servi en 017).
7. **Gardés, avec raison** : les en-têtes de réponse de la passerelle ne sont pas filtrés (elle n'en pose aucun qui
   gêne) ; les en-têtes de requête du navigateur sont relayés sauf ceux retirés (liste de refus : la passerelle ne lit
   aucun des autres) ; l'origine reste contrôlée en double dans la garde du BFF et le proxy (quatre lignes).
