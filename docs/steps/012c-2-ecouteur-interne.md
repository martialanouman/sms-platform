# Step 012c-2 — Écouteur interne du JWKS et déploiement

Plan : `docs/plan/composants.md` §012, seconde des deux PR de 012c (coupe : arbitrage 165). Arbitrages antérieurs : 1 à
172 ; ceux de cette PR sont tranchés en 012c-1 (165, 167, 172 et, pour l'écouteur, la réponse H de Fable du
2026-10-21). Spec : `docs/spec/08-dashboard.md` §8.4 ; `docs/design/06-confiance-interne.md` §7, §9 ; ADR-0019. Règles :
`.claude/rules/tests.md`, `.claude/rules/ci-deploy.md`. Mode : PR.

## Contexte

- 012c-1 a livré le signataire Transit, l'émetteur, `issueGatewayToken` (503 sans signataire) et le handler du JWKS,
  qu'aucun écouteur ne sert.
- `internal/netpol` ne connaît d'écouteur interne que `GRPC_ADDR` ; `deploy/guard_test.go` n'admet un port de Service
  que de `GRPC_ADDR` ou `HTTP_ADDR`, et ne garde l'allowlist que de `content-key-svc`.
- `openbao.sh` est joué par l'humain sous un jeton racine (010a, arbitrage 8).

**Terminé quand** : sur l'environnement de test, `POST /api/gateway-token` d'un `Propriétaire` élevé rend un JWT dont le
`scope` est l'union attendue, vérifié par le JWKS lu sous mTLS depuis un pod porteur d'un certificat `admin-api-svc` ;
sans certificat admis, la poignée de main échoue ; le JWKS est absent de l'écouteur public.

## Design arrêté

- **`internal/platform/config`** : `INTERNAL_ADDR`.
- **`internal/platform/openbao`** : `HTTPClient(ca)`, le client HTTP vers OpenBao (racines de la CA interne, TLS 1.3),
  repris de `cmd/content-key-svc`, que les deux `cmd/` appellent.
- **`internal/dashboard/http`** : `JWKSCallers` (`admin-api-svc`), la seule liste des appelants admis, que lisent le
  démarrage et la garde des manifestes.
- **`cmd/dashboard`** : `INTERNAL_ADDR`, `OPENBAO_ADDR` (`https://`), `TLS_CERT_FILE`, `TLS_KEY_FILE` et
  `TLS_ALLOWED_CLIENTS` (égale à `JWKSCallers`) exigés ; signataire Transit sous le rôle `dashboard` et le jeton
  projeté ; écouteur interne `mtls.Files.Server(callers, ["http/1.1"])` qui ne sert que le JWKS, dans le superviseur.
  Le démarrage n'attend pas OpenBao : le JWKS et `issueGatewayToken` rendent 503 tant qu'il manque.
- **`internal/netpol`** : `INTERNAL_ADDR` ouvre l'ingress à `TLS_ALLOWED_CLIENTS`, comme `GRPC_ADDR` ; l'un des deux
  exige l'allowlist et réciproquement.
- **`deploy/guard_test.go`** : un port de Service peut être celui d'`INTERNAL_ADDR` ; l'allowlist du dashboard est
  `JWKSCallers`.
- **`deploy/bricks/openbao.sh`** : clé `dashboard-token` (`ecdsa-p256`), politique `dashboard` (`update` sur
  `transit/sign/dashboard-token`, `read` sur `transit/keys/dashboard-token`), rôle Kubernetes `dashboard`.
- **`deploy/bricks/openbao.yaml`** : la NetworkPolicy d'OpenBao admet `dashboard`.
- **`deploy/bricks/ca.yaml`** : Certificate `sms/dashboard-tls` (SAN `dashboard`, `server auth`) ; `deploy.sh` l'attend.
- **`deploy/k8s/dashboard.yaml`** : les variables, le Secret `dashboard-tls`, le jeton projeté d'audience `openbao`, le
  port `internal` du Service ; `networkpolicies.yaml` régénéré.
- **Tests** : démarrage refusé sans chacune des variables ou avec une allowlist autre ; écouteur interne réel sous mTLS
  (`mtlstest`) : JWKS servi à `admin-api-svc`, poignée de main refusée à un autre certificat et sans certificat, rien
  d'autre servi ; générateur et gardes sur leurs cas refusés. Mutation manuelle, consignée.
- **Preuve sur `contabo169`** : l'humain rejoue `openbao.sh` ; un pod porteur d'un certificat `admin-api-svc` (émis pour
  la preuve, retiré ensuite) lit le JWKS et vérifie un jeton personnel.
