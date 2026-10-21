# Step 012c-1 — Émetteur, Transit, jeton personnel

Plan : `docs/plan/composants.md` §012, première des deux PR de 012c (coupe : arbitrage 165). Arbitrages antérieurs : 1
à 164. Spec : `docs/spec/08-dashboard.md` §8.4 ; ADR-0019 ; `docs/design/06-confiance-interne.md` §7, §9. Contrat :
`contracts/openapi-bff.yaml` (`issueGatewayToken`), `contracts/openapi-admin.yaml` (`OperatorBearer`). Origine :
`martialanouman/go-gateway-bo`, `internal/token`. Règles : `.claude/rules/tests.md`. Mode : PR.

## Contexte

- Le client OpenBao (login Kubernetes, appel, jeton mis en cache) vit dans `internal/contentkey/openbao`, adaptateur
  d'un autre contexte.
- `issueGatewayToken` est au contrat, sous `gateway:personal_token`, que seul `Propriétaire` détient par défaut ; la
  garde de 012a exige déjà une session élevée.
- Le `CHECK` du journal porte déjà `personal_token_issued`.

**Terminé quand** : contre OpenBao en conteneur, `POST /api/gateway-token` d'une session élevée rend un JWT ES256 dont
le `scope` est l'union attendue et que vérifie le JWKS rendu de la même clé Transit ; le JWKS n'est pas routé par
l'écouteur public.

## Arbitrages

Fable, 2026-10-21.

165. **Coupe de 012c** : 012c-1, cœur et adaptateurs, prouvée en test contre OpenBao réel ; 012c-2, écouteur interne
     mTLS du JWKS, câblage, générateur de NetworkPolicy (`INTERNAL_ADDR`), `openbao.sh`, Certificate, manifeste, preuve
     depuis un pod porteur d'un certificat `admin-api-svc`.
166. **Client partagé** : `internal/platform/openbao` porte le login Kubernetes, l'appel et le jeton mis en cache
     (sémaphore de 1) ; `internal/contentkey/openbao` n'y garde qu'`Encrypt` et `Decrypt` ; `internal/dashboard/openbao`
     y pose la signature et la lecture des clés publiques.
167. **Clé Transit** `dashboard-token`, `ecdsa-p256`, défauts gardés (non exportable, non supprimable). Signature par
     `transit/sign/dashboard-token`, entrée le base64 de `<en-tête>.<charge>`, `hash_algorithm=sha2-256`,
     `marshaling_algorithm=jws` : la sortie est la signature ES256 (`r‖s`, base64url). Politique `dashboard` : `update`
     sur `transit/sign/dashboard-token`, `read` sur `transit/keys/dashboard-token` (posée en 012c-2).
168. **`kid`** = `dashboard-token-v<version Transit>`, jamais réécrit. Le JWKS publie toutes les versions que
     `transit/keys` rend ; JWK `EC`, `P-256`, `x`, `y`, `alg ES256`, `use sig`, rendu par la bibliothèque standard depuis
     le PEM. Aucune bibliothèque JOSE.
169. **Claims** : `iss` = `DASHBOARD_PUBLIC_URL` telle quelle, `aud` = `gateway-admin`, `sub`, `scope` (scopes triés,
     séparés par une espace), `iat`, `exp`. Pas de `jti`. Le jeton de 5 min et sa réutilisation viennent en 013, avec le
     proxy.
170. **Jeton personnel** : 1 h ; scopes demandés inclus dans (union de l'opérateur ∩ `scopes.All`) − {`content:read`,
     `content:erase`, `gdpr:erase`}, sinon 403 `forbidden_scope` nommant le premier refusé ; sans `scopes`, tout ce qui
     reste ; ensemble vide → 403. `GatewayToken.scopes` = l'ensemble effectif, trié, identique au claim. Journal
     `personal_token_issued`, acteur l'opérateur, rien du jeton. OpenBao injoignable ou refusant → 503. Rien n'est
     stocké.
171. **Élévation** : la garde de 012a suffit (l'ADR exige la MFA, pas une MFA récente).
172. **JWKS** : lu dans Transit à chaque requête, sans cache côté BFF ; `Cache-Control: max-age=60` ; OpenBao en panne →
     503. Il n'est pas routé par l'écouteur public (test bloquant, design 06 §9).

## Design arrêté

- **`internal/platform/openbao`** : `Client` (login Kubernetes, `Call`, renouvellement, 403 qui jette le jeton),
  extrait de `contentkey/openbao` sans changement de comportement ; ses tests suivent.
- **Cœur `internal/dashboard`** : `token.go` (port `Signer` : `Sign(ctx, input) (signature, kid)`, `PublicKeys(ctx)` ;
  `IssuePersonalToken` ; rendu du JWT et du JWKS) ; `ErrSignerUnavailable`.
- **`internal/dashboard/openbao`** : `Signer` sur le client partagé.
- **`internal/dashboard/http`** : `issueGatewayToken` rejoint `include-operation-ids` ; un `http.Handler` du JWKS,
  exporté pour 012c-2, que `New` ne route pas.
- **Tests** : cœur (rendu du JWT, sélection des scopes, journal) ; adaptateur contre OpenBao en conteneur (signature
  vérifiée par `ecdsa.Verify` avec la clé du JWKS, rotation : deux versions, deux `kid`) ; HTTP (403, 503, JWKS absent
  de l'écouteur public) ; `contentkey` inchangé et vert. Mutation manuelle, consignée.
- **Preuve** : en test seulement ; la preuve sur le cluster vient en 012c-2.
