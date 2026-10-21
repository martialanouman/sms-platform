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
